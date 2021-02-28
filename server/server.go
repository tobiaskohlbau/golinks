package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"text/template"

	"github.com/go-chi/chi"
	bolt "go.etcd.io/bbolt"
)

type server struct {
	r       chi.Router
	db      *bolt.DB
	content fs.FS
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.r.ServeHTTP(w, r)
}

func New(db *bolt.DB, content fs.FS) http.Handler {
	srv := &server{
		r:       chi.NewRouter(),
		db:      db,
		content: content,
	}
	srv.Routes()
	return srv
}

type Entry struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

func (s *server) handleRegistry() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tpl, err := template.ParseFS(s.content, "templates/registry.tmpl")
		if err != nil {
			http.Error(w, fmt.Errorf("failed to find template: %w", err).Error(), http.StatusInternalServerError)
			return
		}

		var entries []Entry
		err = s.db.View(func(tx *bolt.Tx) error {
			bucket := tx.Bucket([]byte("redirects"))
			c := bucket.Cursor()

			for k, v := c.First(); k != nil; k, v = c.Next() {
				entries = append(entries, Entry{
					Source:      string(k),
					Destination: string(v),
				})
			}

			return nil
		})
		if err != nil {
			http.Error(w, fmt.Errorf("failed to view entries: %w", err).Error(), http.StatusInternalServerError)
			return
		}

		if err := tpl.Execute(w, struct {
			Entries []Entry
		}{
			Entries: entries,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (s *server) handleEdit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			r.URL.Path = r.URL.Path + "/%s"
			http.Redirect(w, r, r.URL.String(), http.StatusTemporaryRedirect)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/edit/")
		destination := ""
		s.db.View(func(tx *bolt.Tx) error {
			destination = string(tx.Bucket([]byte("redirects")).Get([]byte(key)))
			return nil
		})
		tpl, err := template.ParseFS(s.content, "templates/edit.tmpl")
		if err != nil {
			http.Error(w, fmt.Errorf("failed to find template: %w", err).Error(), http.StatusInternalServerError)
			return
		}
		if err := tpl.Execute(w, struct {
			Path        string
			Destination string
		}{
			Path:        key,
			Destination: destination,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (s *server) handleSave() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var entry Entry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			http.Error(w, fmt.Errorf("failed to decode save request: %w", err).Error(), http.StatusBadRequest)
			return
		}

		err := s.db.Update(func(tx *bolt.Tx) error {
			bucket := tx.Bucket([]byte("redirects"))
			if entry.Destination == "" {
				return bucket.Delete([]byte(entry.Source))
			}
			return bucket.Put([]byte(entry.Source), []byte(entry.Destination))
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (s *server) handleRedirect() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := s.db.View(func(tx *bolt.Tx) error {
			path := strings.TrimPrefix(r.URL.Path, "/")
			bucket := tx.Bucket([]byte("redirects"))

			// check for exact match
			entry := bucket.Get([]byte(path))
			if entry != nil {
				http.Redirect(w, r, string(entry), http.StatusTemporaryRedirect)
				return nil
			}

			// check for wildcard
			paths := strings.Split(path, "/")
			position := -1
			for i := 0; i < len(paths); i++ {
				elements := []string{}
				if len(paths[:i]) > 0 {
					elements = append(elements, paths[:i]...)
				}
				elements = append(elements, "%s")
				if len(paths[i+1:]) > 0 {
					elements = append(elements, paths[i+1:]...)
				}
				checkURL := strings.Join(elements, "/")
				entry = bucket.Get([]byte(checkURL))
				if entry != nil {
					position = i
					break
				}
			}
			if entry == nil {
				r.URL.Path = "/edit/" + path
				http.Redirect(w, r, r.URL.String(), http.StatusTemporaryRedirect)
				return nil
			}
			url := string(entry)
			if strings.Contains(url, "%s") {
				url = strings.Replace(url, "%s", paths[position], -1)
			}

			http.Redirect(w, r, url, http.StatusTemporaryRedirect)
			return nil
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}
}
