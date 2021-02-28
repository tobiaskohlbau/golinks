package main

import (
	"embed"
	"encoding/csv"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi"
	"github.com/tobiaskohlbau/golinks/server"
	bolt "go.etcd.io/bbolt"
)

//go:embed static templates
var contentEmbedded embed.FS

func importCSV(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open import file: %w", err)
	}
	defer file.Close()
	r := csv.NewReader(file)
	r.Comma = ';'
	records, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read import file: %w", err)
	}

	db, err := bolt.Open("registry.db", 0600, nil)
	if err != nil {
		return fmt.Errorf("failed to open registry db: %w", err)
	}
	defer db.Close()

	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("redirects"))
		if err != nil {
			return fmt.Errorf("failed to create redirects bucket: %w", err)
		}

		for _, record := range records {
			if len(record) != 2 {
				return fmt.Errorf("can't import bad line: %s", record)
			}
			if err := bucket.Put([]byte(record[0]), []byte(record[1])); err != nil {
				return fmt.Errorf("failed to import redirect: %s->%s: %w", record[0], record[1], err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to prepare registry db: %w", err)
	}

	return nil
}

func execute() error {
	dev := flag.Bool("dev", false, "enable development mode")
	flag.Parse()

	if len(os.Args) > 1 && os.Args[1] == "import" {
		return importCSV(os.Args[2])
	}

	var content fs.FS = contentEmbedded
	if *dev {
		content = os.DirFS(".")
	}

	db, err := bolt.Open("registry.db", 0600, nil)
	if err != nil {
		return fmt.Errorf("failed to open registry db: %w", err)
	}
	defer db.Close()

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("redirects"))
		if err != nil {
			return fmt.Errorf("failed to create redirects bucket: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to prepare registry db: %w", err)
	}

	r := chi.NewRouter()
	r.Handle("/static/*", http.FileServer(http.FS(content)))

	srv := server.New(db, content)
	r.Mount("/", srv)

	return http.ListenAndServe(":80", r)
}

func main() {
	if err := execute(); err != nil {
		log.Fatal(err)
	}
}
