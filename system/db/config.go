package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/bosssauce/ponzu/system/admin/config"

	"github.com/boltdb/bolt"
	"github.com/gorilla/schema"
)

var configCache url.Values

func init() {
	configCache = make(url.Values)
}

// SetConfig sets key:value pairs in the db for configuration settings
func SetConfig(data url.Values) error {
	fmt.Println("SetConfig:", data)
	err := store.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("__config"))

		// check for any multi-value fields (ex. checkbox fields)
		// and correctly format for db storage. Essentially, we need
		// fieldX.0: value1, fieldX.1: value2 => fieldX: []string{value1, value2}
		var discardKeys []string
		for k, v := range data {
			if strings.Contains(k, ".") {
				key := strings.Split(k, ".")[0]

				if data.Get(key) == "" {
					data.Set(key, v[0])
					discardKeys = append(discardKeys, k)
				} else {
					data.Add(key, v[0])
				}
			}
		}

		for _, discardKey := range discardKeys {
			data.Del(discardKey)
		}

		cfg := &config.Config{}
		dec := schema.NewDecoder()
		dec.SetAliasTag("json")     // allows simpler struct tagging when creating a content type
		dec.IgnoreUnknownKeys(true) // will skip over form values submitted, but not in struct
		err := dec.Decode(cfg, data)
		if err != nil {
			return err
		}

		// check for "invalidate" value to reset the Etag
		if len(cfg.CacheInvalidate) > 0 && cfg.CacheInvalidate[0] == "invalidate" {
			cfg.Etag = NewEtag()
			cfg.CacheInvalidate = []string{}
		}

		j, err := json.Marshal(cfg)
		if err != nil {
			return err
		}

		err = b.Put([]byte("settings"), j)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	configCache = data

	return nil
}

// Config gets the value of a key in the configuration from the db
func Config(key string) ([]byte, error) {
	kv := make(map[string]interface{})

	cfg, err := ConfigAll()
	if err != nil {
		return nil, err
	}

	if len(cfg) < 1 {
		return nil, nil
	}

	err = json.Unmarshal(cfg, &kv)
	if err != nil {
		return nil, err
	}

	return []byte(kv[key].(string)), nil
}

// ConfigAll gets the configuration from the db
func ConfigAll() ([]byte, error) {
	val := &bytes.Buffer{}
	err := store.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("__config"))
		val.Write(b.Get([]byte("settings")))

		return nil
	})
	if err != nil {
		return nil, err
	}

	return val.Bytes(), nil
}

// PutConfig updates a single k/v in the config
func PutConfig(key string, value interface{}) error {
	fmt.Println("PutConfig:", key, value)
	kv := make(map[string]interface{})

	c, err := ConfigAll()
	if err != nil {
		return err
	}

	err = json.Unmarshal(c, &kv)
	if err != nil {
		return err
	}

	data := make(url.Values)
	for k, v := range kv {
		switch v.(type) {
		case string:
			data.Set(k, v.(string))

		case []string:
			vv := v.([]string)
			for i := range vv {
				data.Add(k, vv[i])
			}

		default:
			log.Println("No type case for:", k, v, "in PutConfig")
			data.Set(k, fmt.Sprintf("%v", v))
		}
	}

	fmt.Println("data should match 2 lines below:")
	fmt.Println("PutConfig:", data)

	err = SetConfig(data)
	if err != nil {
		return err
	}

	return nil
}

// ConfigCache is a in-memory cache of the Configs for quicker lookups
// 'key' is the JSON tag associated with the config field
func ConfigCache(key string) string {
	return configCache.Get(key)
}
