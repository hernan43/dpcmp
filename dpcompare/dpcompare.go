package dpcompare

import (
  "appengine"
  "appengine/datastore"
  "appengine/user"
  "bytes"
  "fmt"
  "http"
  "image"
  "image/jpeg"
  _ "image/png" // import so we can read PNG files
  "io"
  "os"
  "resize"
  "crypto/sha1"
  "template"
)

// Image is the type used to hold the image in the datastore.
type Comparison struct {
        Left []byte
        Right []byte
        Submitter string
}


func (c *Comparison) key() string {
  sha := sha1.New()
  // use everything to create the hash
  sha.Write(c.Left)
  sha.Write(c.Right)
  sha.Write([]byte(c.Submitter))
  return fmt.Sprintf("%x", string(sha.Sum())[0:16])
}

var templates = make(map[string]*template.Template)

// check aborts the current execution if err is non-nil.
// stolen from 
// http://goo.gl/lgkTC
func check(err os.Error) {
  if err != nil {
    panic(err)
  }
}

// 99.9% of this was stolen from 
// http://goo.gl/lgkTC
func extractImageFromPost(name string, r *http.Request) []byte {
  f, _, err := r.FormFile(name)
  check(err)
  defer f.Close()

  // Grab the image data
  buf := new(bytes.Buffer)
  io.Copy(buf, f)
  i, _, err := image.Decode(buf)
  check(err)

  // We aim for less than 800 pixels in any dimension; if the
  // picture is larger than that, we squeeze it down
  const max = 800
  if b := i.Bounds(); b.Dx() > max || b.Dy() > max {
    // If it's gigantic, it's more efficient to downsample first
    // and then resize; resizing will smooth out the roughness.
    if b.Dx() > 2*max || b.Dy() > 2*max {
      w, h := max, max
      if b.Dx() > b.Dy() {
              h = b.Dy() * h / b.Dx()
      } else {
              w = b.Dx() * w / b.Dy()
      }
      i = resize.Resample(i, i.Bounds(), w, h)
      b = i.Bounds()
    }
    w, h := max/2, max/2
    if b.Dx() > b.Dy() {
            h = b.Dy() * h / b.Dx()
    } else {
            w = b.Dx() * w / b.Dy()
    }
    i = resize.Resize(i, i.Bounds(), w, h)
  }

  // Encode as a new JPEG image.
  buf.Reset()
  err = jpeg.Encode(buf, i, nil)
  check(err)

  // return JPEG
  return buf.Bytes()
}

func makeHandler(fn func(http.ResponseWriter, *http.Request, appengine.Context, *user.User)) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    c := appengine.NewContext(r)
    u := user.Current(c)
    if u == nil {
      url, err := user.LoginURL(c, r.URL.String())
      if err != nil {
          http.Error(w, err.String(), http.StatusInternalServerError)
          return
      }
      w.Header().Set("Location", url)
      w.WriteHeader(http.StatusFound)
      return
    }
    fn(w, r, c, u)
  }
}

func renderTemplate(w http.ResponseWriter, tmpl string, m map[string] string) {
  err := templates[tmpl].Execute(w, m)
  if err != nil {
    http.Error(w, err.String(), http.StatusInternalServerError)
  }
}

func init() {
  /*cache templates*/
  for _, tmpl := range []string{"index", "show"} {
    templates[tmpl] = template.MustParseFile(tmpl+".html", nil)
  }

  /*setup handlers*/
  http.HandleFunc("/", makeHandler(index))
  http.HandleFunc("/upload", makeHandler(upload))
  http.HandleFunc("/show", makeHandler(show))
  http.HandleFunc("/img", makeHandler(img))
}

func index(w http.ResponseWriter, r *http.Request, c appengine.Context, u *user.User) {
  templateContext := make(map[string] string)
  templateContext["username"] = u.Email
  // render
  renderTemplate(w, "index", templateContext)
}

func upload(w http.ResponseWriter, r *http.Request, c appengine.Context, u *user.User) {
  templateContext := make(map[string] string)
  if r.Method != "POST" {
    templateContext["username"] = u.Email
    renderTemplate(w, "index", templateContext)
    return
  }

  comparison := new(Comparison)
  comparison.Left = extractImageFromPost("left_picture", r)
  comparison.Right = extractImageFromPost("right_picture", r)
  comparison.Submitter = u.Email

  // Save the comparison under a unique key, a hash of the struct's data
  key := datastore.NewKey("Comparison", comparison.key(), 0, nil)
  _, err := datastore.Put(c, key, comparison)
  check(err)

  // Redirect to /edit using the key.
  http.Redirect(w, r, "/show?id="+key.StringID(), http.StatusFound)
}

// it handles "/img".
func img(w http.ResponseWriter, r *http.Request, c appengine.Context, u *user.User) {
  key := datastore.NewKey("Comparison", r.FormValue("id"), 0, nil)
  side := r.FormValue("side")
  comparison := new(Comparison)
  err := datastore.Get(c, key, comparison)
  check(err)

  if( side == "left"){
    m, _, err := image.Decode(bytes.NewBuffer(comparison.Left))
  } else {
    m, _, err := image.Decode(bytes.NewBuffer(comparison.Right))
  }
  check(err)

  w.Header().Set("Content-type", "image/jpeg")
  jpeg.Encode(w, m, nil)
}

func show(w http.ResponseWriter, r *http.Request, c appengine.Context, u *user.User) {
  templateContext := make(map[string] string)
  templateContext["username"] = u.Email

  key := r.FormValue("id")
  // render
  renderTemplate(w, "show", templateContext)
}

