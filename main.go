package main

import (
	"context"
	"embed"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/MicahParks/keyfunc"
	"github.com/go-chi/chi"
	"github.com/golang-jwt/jwt/v4"
	"github.com/tobiaskohlbau/golinks/server"
	bolt "go.etcd.io/bbolt"
)

//go:embed static templates
var contentEmbedded embed.FS

func importCSV(dbPath string, path string) error {
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

	db, err := bolt.Open(dbPath, 0600, nil)
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
	dbPath := flag.String("data", "/data/registry.db", "path to store database")
	jwksURL := flag.String("jwksURL", "", "JWKS endpoint.")
	jwtHeaderName := flag.String("jwtHeaderName", "", "JWT Headername")
	jwtClaimName := flag.String("jwtClaimName", "", "JWT Clainname")
	flag.Parse()

	if len(os.Args) > 1 && os.Args[1] == "import" {
		return importCSV(*dbPath, os.Args[2])
	}

	var content fs.FS = contentEmbedded
	if *dev {
		content = os.DirFS(".")
	}

	db, err := bolt.Open(*dbPath, 0600, nil)
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
	if *jwksURL != "" {
		r.Use(jwtHandler(db, *jwksURL, *jwtHeaderName, *jwtClaimName))
	}
	r.Handle("/static/*", http.FileServer(http.FS(content)))

	srv := server.New(db, content)
	r.Mount("/", srv)

	return http.ListenAndServe(":80", r)
}

func jwtHandler(db *bolt.DB, jwksURL string, headerName string, claimName string) func(http.Handler) http.Handler {
	jwks, err := keyfunc.Get(jwksURL, keyfunc.Options{})
	if err != nil {
		panic(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("users"))
		if err != nil {
			return fmt.Errorf("failed to create users bucket: %w", err)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jwtBase64 := r.Header.Get(headerName)

			if jwtBase64 == "" {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			token, err := jwt.Parse(jwtBase64, jwks.Keyfunc)
			if err != nil {
				log.Println(err)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			claims := token.Claims.(jwt.MapClaims)
			if err := claims.Valid(); err != nil {
				log.Println(err)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}
			username, ok := claims[claimName].(string)
			if !ok {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}

			var user User
			err = db.Update(func(tx *bolt.Tx) error {
				bucket := tx.Bucket([]byte("users"))

				userData := bucket.Get([]byte(username))
				if userData == nil {
					user = User{
						Username: username,
					}
					data, err := json.Marshal(user)
					if err != nil {
						return fmt.Errorf("failed to marshal new user: %w", err)
					}
					err = bucket.Put([]byte(username), data)
					if err != nil {
						return fmt.Errorf("failed to store new user: %w", err)
					}
				} else {
					err := json.Unmarshal(userData, &user)
					if err != nil {
						return fmt.Errorf("failed to unmarshal stored user: %w", err)
					}
				}

				return nil
			})

			if err != nil {
				log.Println(err)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), "user", user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type User struct {
	Username string
}

func main() {
	if err := execute(); err != nil {
		log.Fatal(err)
	}
}
