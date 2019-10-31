package table

import (
	"github.com/quan8/go-ethereum/ethdb"
	"reflect"
)

// MigrateTables sets target fields to database tables.
func MigrateTables(s interface{}, db ethdb.KeyValueStore) {
	value := reflect.ValueOf(s).Elem()
	for i := 0; i < value.NumField(); i++ {
		if prefix := value.Type().Field(i).Tag.Get("table"); prefix != "" && prefix != "-" {
			field := value.Field(i)
			var val reflect.Value
			if db != nil {
				table := New(db, []byte(prefix))
				val = reflect.ValueOf(table)
			} else {
				val = reflect.Zero(field.Type())
			}
			field.Set(val)
		}
	}
}

// MigrateCaches sets target fields to get() result.
func MigrateCaches(c interface{}, get func() interface{}) {
	value := reflect.ValueOf(c).Elem()
	for i := 0; i < value.NumField(); i++ {
		if prefix := value.Type().Field(i).Tag.Get("cache"); prefix != "" {
			field := value.Field(i)
			var cache interface{}
			if get != nil {
				cache = get()
			}
			var val reflect.Value
			if cache != nil {
				val = reflect.ValueOf(cache)
			} else {
				val = reflect.Zero(field.Type())
			}
			field.Set(val)
		}
	}
}
