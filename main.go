package main

import (
	"encoding/csv"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi"
	bolt "go.etcd.io/bbolt"
)

var tpl = template.Must(template.New("tpl").Parse(`
<!doctype html>

<html lang="en">
<head>
  <meta charset="utf-8">

  <title>{{ .Path }}</title>
  <meta name="description" content="{{ .Path }}">
	<style>
		html, body {
			height: 100%;
		}
		body {
			margin: 0;
		}
	  .container {
			display: flex;
			height: 100%;
			width: 100%;
			align-items: center;
			justify-content: center;
		}
		#destination {
			font-size: 22px;
			height: 48px;
			width: 800px;
		}
	</style>

</head>

<body>
	<div class="container">
		<input type="text" id="destination" onkeyup="processChange()" value="{{ .Destination }}"/>
	</div>
	<script>
	function debounce(func, timeout = 2000){
		let timer;
		return (...args) => {
			clearTimeout(timer);
			timer = setTimeout(() => { func.apply(this, args); }, timeout);
		};
	}
	function saveInput(){
			var element = document.getElementById("destination");
			var req = new XMLHttpRequest();
			req.open("POST", encodeURI("/edit/{{ .Path }}"), true);
			req.setRequestHeader("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8");
			req.send("destination="+encodeURI(element.value));
	}
	const processChange = debounce(() => saveInput());
	</script>
</body>
</html>
`))

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
	if os.Args[1] == "import" {
		return importCSV(os.Args[2])
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
	r.Get("/edit/*", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			r.URL.Path = r.URL.Path + "/%s"
			http.Redirect(w, r, r.URL.String(), http.StatusTemporaryRedirect)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/edit/")
		destination := ""
		db.View(func(tx *bolt.Tx) error {
			destination = string(tx.Bucket([]byte("redirects")).Get([]byte(key)))
			return nil
		})
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
	})
	r.Post("/edit/*", func(w http.ResponseWriter, r *http.Request) {
		dest := r.FormValue("destination")
		url := strings.TrimPrefix(r.URL.Path, "/edit/")
		err := db.Update(func(tx *bolt.Tx) error {
			bucket := tx.Bucket([]byte("redirects"))
			if dest == "" {
				return bucket.Delete([]byte(url))
			}
			return bucket.Put([]byte(url), []byte(dest))
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		err := db.View(func(tx *bolt.Tx) error {
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
	})
	return http.ListenAndServe(":80", r)
}

func main() {
	if err := execute(); err != nil {
		log.Fatal(err)
	}
}
