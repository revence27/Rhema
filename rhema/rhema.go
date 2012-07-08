package main

import (
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
)

var port int
var hostname, defaultVersion, defaultBook, defaultChapter string
var session *mgo.Session
var bible *mgo.Database
var page *template.Template

type VerseMap map[string]interface{}
type VersionMap map[string]interface{}
type BookMap map[string]interface{}

func (book *BookMap) Name() template.HTML {
	return template.HTML((*book)["name"].([]byte))
}

func (version *VersionMap) Name() template.HTML {
	return template.HTML((*version)["name"].([]byte))
}

func (verse *VerseMap) Rhema() template.HTML {
	return template.HTML((*verse)["rhema"].([]byte))
}

func (verse *VerseMap) Position() int {
	return (*verse)["position"].(int)
}

func failWell(rw http.ResponseWriter, req *http.Request) {
	got := recover()
	if got == nil {
		return
	}
	fmt.Fprintf(rw, "%v\n", got)
}

func squealError(e error, stat, msg string, rw http.ResponseWriter, req *http.Request, nxt func() bool) bool {
	if e != nil {
		log.Printf("[%s] %v", req.URL.String(), e)
		rw.Header().Set("Status", fmt.Sprintf("400 %s", stat))
		e = page.Execute(rw, struct {
			ForError     bool
			ErrorMessage []byte
		}{
			ForError:     true,
			ErrorMessage: []byte(fmt.Sprintf("%s %v", msg, e)),
		})
		if e != nil {
			panic(e)
		}
		return false
	}
	return nxt()
}

func rhemaHandler(rw http.ResponseWriter, req *http.Request) {
	defer failWell(rw, req)

	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	books := bible.C("books")
	chapters := bible.C("chapters")
	verses := bible.C("verses")
	versions := bible.C("versions")
	book := make(map[string]interface{})
	vname := []byte(req.FormValue("t"))
	bname := []byte(req.FormValue("b"))
	ver := make(map[string]interface{})
	if req.FormValue("t") == "" {
		vname = []byte(defaultVersion)
	}
	squealError(versions.Find(bson.M{
		"name": vname,
	}).Limit(1).One(ver), "No such version", fmt.Sprintf("No translation called \"%s\"", vname), rw, req, func() bool {
		if req.FormValue("b") == "" {
			bname = []byte(defaultBook)
		}
		squealError(books.Find(bson.M{
			"name":    bname,
			"version": ver["_id"],
		}).Limit(1).One(book), "No book by that name.", fmt.Sprintf("No book by the name \"%s\" in \"%s\"", bname, vname), rw, req, func() bool {
			chapter := make(map[string]interface{})
			cposs := req.FormValue("c")
			if cposs == "" {
				cposs = defaultChapter
			}
			cpos, e := strconv.Atoi(cposs)
			if e != nil {
				log.Printf("[%s] %v", req.URL.String(), e)
				cpos = 1
			}
			squealError(chapters.Find(bson.M{
				"position": cpos,
				"book":     book["_id"],
			}).Limit(1).One(chapter), fmt.Sprintf("That book has no chapter number %d.", cpos), fmt.Sprintf("\"%s\" has no chapter \"%s\"", bname, cposs), rw, req, func() bool {
				them := make([]VerseMap, 0)
				squealError(verses.Find(bson.M{
					"chapter": chapter["_id"],
				}).Sort("position").All(&them), fmt.Sprintf("Could not fetch verses for chapter %d.", cpos), fmt.Sprintf("Failure to fetch verses for \"%s\" chapter %d", bname, cpos), rw, req, func() bool {
					vers := make([]VersionMap, 0)
					squealError(versions.Find(nil).Sort("name").All(&vers), "No versions ready", "No versions ready", rw, req, func() bool {
						bkm := make([]string, 0)
						//  squealError(books.Find(nil).Sort("name").All(&bkm), "No books ready", "No books ready", rw, req, func () bool {
						squealError(books.Find(nil).Sort("name").Distinct("name", &bkm), "No books ready", "No books ready", rw, req, func() bool {
							e = page.Execute(rw, struct {
								ForError bool
								Book     []byte
								Chapter  int
								Verses   []VerseMap
								Versions []VersionMap
								Books    []string
							}{
								ForError: false,
								Book:     book["name"].([]byte),
								Chapter:  chapter["position"].(int),
								Verses:   them,
								Versions: vers,
								Books:    bkm,
							})
							if e != nil {
								panic(e)
							}
							return true
						})
						return true
					})
					return true
				})
				return true
			})
			return true
		})
		return true
	})
}

func main() {
	var chn chan error
	go func() {
		chn <- http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	}()
	log.Printf("Rhema on port %d\n", port)
	panic(<-chn)
}

func init() {
	flag.StringVar(&hostname, "host", "localhost", "Mongo DB host details.")
	flag.StringVar(&defaultVersion, "version", "New International Version (UK)", "Default Bible version to use, if none provided in request.")
	flag.StringVar(&defaultBook, "book", "Romans", "Default book to open, if none provided in request.")
	flag.StringVar(&defaultChapter, "chapter", "5", "Default chapter to open, if none provided in request.")
	flag.IntVar(&port, "port", 8998, "Port on which to bind the Rhema server.")
	var templatePath string
	flag.StringVar(&templatePath, "template", path.Join(path.Dir(os.Args[0]), "rhema.html"), "Path to the template file to use in generating web pages.")
	flag.Parse()
	if !flag.Parsed() {
		panic("Arguments could not be parsed.")
	}
	dat, e := ioutil.ReadFile(templatePath)
	if e != nil {
		panic(e)
	}
	page, e = template.New("Rhema").Parse(string(dat))
	if e != nil {
		panic(e)
	}
	session, e := mgo.Dial(hostname)
	if e != nil {
		panic(e)
	}
	bible = session.DB("bible")
	http.HandleFunc("/", rhemaHandler)
}
