package main

import (
  "encoding/xml"
  "flag"
  "fmt"
  "io/ioutil"
  "labix.org/v2/mgo"
  "labix.org/v2/mgo/bson"
  "os"
  "path"
  "strings"
)

func errorReporter() {
  ep  :=  recover()
  if ep == nil {  return  }
  fmt.Fprintf(os.Stderr, "%v\n", ep)
  os.Exit(1)
}

type Verse struct {
  Rhema     []byte          `xml:",innerxml"`
  Position  int             `xml:"vnumber,attr"`
}

type Chapter struct {
  Verses    []Verse         `xml:"VERS"`
  Position  int             `xml:"cnumber,attr"`
}

type Book struct {
  Chapters  []Chapter       `xml:"CHAPTER"`
  Name      []byte          `xml:"bname,attr"`
  Position  int             `xml:"bnumber,attr"`
}

type Bible struct {
  Books     []Book          `xml:"BIBLEBOOK"`
  Version   []byte          `xml:"biblename,attr"`
}

func idempotentRecordVersion(bible *Bible, col *mgo.Collection) interface{} {
  versions  :=  col.Find(bson.M{
    "name"  : bible.Version,
  }).Limit(1)
  cpt, e  :=  versions.Count()
  if e != nil { panic(e)  }
  if cpt > 0 {
    ans := make(map[string]interface{})
    e    = versions.One(ans)
    if e != nil { panic(e)  }
    return ans["_id"]
  }
  e = col.Insert(bson.M{
    "name"  : bible.Version,
  })
  if e != nil { panic(e)  }
  return idempotentRecordVersion(bible, col)
}

func idempotentRecordBook(vid interface{}, book Book, col *mgo.Collection) interface{} {
  them  :=  col.Find(bson.M{
    "name"    : book.Name,
    "version" : vid,
  }).Limit(1)
  cpt, e  :=  them.Count()
  if e != nil { panic(e)  }
  if cpt > 0 {
    ans :=  make(map[string]interface{})
    e =  them.One(ans)
    if e != nil { panic(e)  }
    return ans["_id"]
  }
  e = col.Insert(bson.M{
    "name"      : book.Name,
    "position"  : book.Position,
    "version"   : vid,
  })
  if e != nil { panic(e)  }
  return idempotentRecordBook(vid, book, col)
}

func idempotentRecordChapter(bid interface{}, chapter Chapter, col *mgo.Collection) interface{} {
  them  :=  col.Find(bson.M{
    "position"  : chapter.Position,
    "book"      : bid,
  }).Limit(1)
  cpt, e  :=  them.Count()
  if e != nil { panic(e)  }
  if cpt > 0 {
    ans :=  make(map[string]interface{})
    e = them.One(ans)
    if e != nil { panic(e)  }
    return ans["_id"]
  }
  e = col.Insert(bson.M{
    "position"  : chapter.Position,
    "book"      : bid,
  })
  if e != nil { panic(e)  }
  return idempotentRecordChapter(bid, chapter, col)
}

func idempotentRecordVerse(cid interface{}, verse Verse, col *mgo.Collection) {
  _, e  :=  col.Upsert(bson.M{
    "chapter"   : cid,
    "position"  : verse.Position,
  }, bson.M{
    "chapter"   : cid,
    "position"  : verse.Position,
    "rhema"     : verse.Rhema,
  })
  if e != nil { panic(e)  }
}

func recordBible(bible *Bible, ses *mgo.Session) {
  database    :=  ses.DB("bible")
  version     :=  database.C("versions")
  vid         :=  idempotentRecordVersion(bible, version)
  collection  :=  database.C("books")
  for _, book := range bible.Books {
    bid :=  idempotentRecordBook(vid, book, collection)
    ccoll :=  database.C("chapters")
    for _, chapter  := range book.Chapters {
      cid :=  idempotentRecordChapter(bid, chapter, ccoll)
      vcoll :=  database.C("verses")
      for _, verse  :=  range chapter.Verses {
        fmt.Fprintf(os.Stderr, fmt.Sprintf("%s %s %d:%d %s%s", bible.Version, book.Name, chapter.Position, verse.Position, verse.Rhema, strings.Repeat(" ", 80))[:75])
        idempotentRecordVerse(cid, verse, vcoll)
        fmt.Fprintf(os.Stderr, "\r")
      }
      fmt.Fprintf(os.Stderr, "\r")
    }
  }
}

func processXMLPath(it string, ses *mgo.Session) {
  ans, e  :=  ioutil.ReadFile(it)
  if e != nil { panic(e)  }
  bible :=  new(Bible)
  e = xml.Unmarshal(ans, bible)
  if e != nil { panic(e)  }
  bible.Version = []byte(strings.SplitN(path.Base(it), ".", 2)[0])
  recordBible(bible, ses)
}

func main() {
  defer errorReporter()
  var host string
  flag.StringVar(&host, "host", "localhost", "Hostname of Mongo DB to use.")
  flag.Parse()
  p :=  flag.Parsed()
  if !p { panic("Could not parse the command line arguments.") }
  them  :=  flag.Args()
  if len(them) < 1 {  panic("Provide the path to an XML file.") }
  ses, e  :=  mgo.Dial(host)
  defer ses.Close()
  if e != nil { panic(e)  }
  for _, it :=  range them {
    processXMLPath(it, ses)
  }
}
