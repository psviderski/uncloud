package main

import (
	"fmt"
	"github.com/dgraph-io/badger/v3"
	"log"
	"time"
)

func customTimeEncoder(t time.Time) string {
	return t.Format("2006-01-02T15:04:05.000000Z07:00")
}

func main() {
	opts := badger.DefaultOptions("./store")
	opts.Logger = nil // Disable the noisy Badger's default logger

	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 10
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			err := item.Value(func(v []byte) error {
				fmt.Printf("key=%s, value=%s, raw value=%v\n", k, v, v)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
