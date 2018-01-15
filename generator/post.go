package generator

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/russross/blackfriday"
	"github.com/sourcegraph/syntaxhighlight"
	"gopkg.in/yaml.v2"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Post holds data for a post
type Post struct {
	Name      string
	HTML      []byte
	Meta      *Meta
	ImagesDir string
	Images    []string
}

// ByDateDesc is the sorting object for posts
type ByDateDesc []*Post

// PostGenerator object
type PostGenerator struct {
	Config *PostConfig
}

// PostConfig holds the post's configuration
type PostConfig struct {
	Post        *Post
	Destination string
	Template    *template.Template
	Writer      *IndexWriter
}

// Generate generates a post
func (g *PostGenerator) Generate() error {
	post := g.Config.Post
	destination := g.Config.Destination
	t := g.Config.Template
	fmt.Printf("\tGenerating Post: %s...\n", post.Meta.Title)
	staticPath := filepath.Join(destination, post.Name)
	if err := os.Mkdir(staticPath, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directory at %s: %v", staticPath, err)
	}
	if post.ImagesDir != "" {
		if err := copyImagesDir(post.ImagesDir, staticPath); err != nil {
			return err
		}
	}

	if err := g.Config.Writer.WriteIndexHTML(staticPath, post.Meta.Title, post.Meta.Short, template.HTML(string(post.HTML)), t); err != nil {
		return err
	}
	fmt.Printf("\tFinished generating Post: %s...\n", post.Meta.Title)
	return nil
}

func newPost(path, dateFormat string) (*Post, error) {
	filePath := filepath.Join(path, "post.md")
	file, err := os.Open(filePath)
	br := bufio.NewReader(file)
	meta, err := getMeta(br, dateFormat)
	if err != nil {
		return nil, fmt.Errorf(`error parsing meta in %s:%v`, filePath, err)
	}
	html, err := getHTML(br)
	if err != nil {
		return nil, err
	}
	imagesDir, images, err := getImages(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)

	return &Post{Name: name, Meta: meta, HTML: html, ImagesDir: imagesDir, Images: images}, nil
}

func copyImagesDir(source, destination string) (err error) {
	path := filepath.Join(destination, "images")
	if err := os.Mkdir(path, os.ModePerm); err != nil {
		return fmt.Errorf("error creating images directory at %s: %v", path, err)
	}
	files, err := ioutil.ReadDir(source)
	if err != nil {
		return fmt.Errorf("error reading directory %s: %v", path, err)
	}
	for _, file := range files {
		src := filepath.Join(source, file.Name())
		dst := filepath.Join(path, file.Name())
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// Unmarshal the file's header.
func getMeta(br *bufio.Reader, dateFormat string) (*Meta, error) {
	// read first line
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("error ReadString: %v", err)
	}
	// start with ---
	if !strings.HasPrefix(line, "---") {
		err = errors.New(`Can not find "---" `)
		return nil, fmt.Errorf(`error in can not find "---"`)
	}
	buf := bytes.NewBuffer(nil)
	// read header
	for {
		line, err = br.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return nil, fmt.Errorf("error ReadString: %v", err)
			}
		}
		// end of header
		if strings.HasPrefix(line, "---") {
			break
		}
		buf.WriteString(line)
	}
	h, err := ioutil.ReadAll(buf)
	if err != nil || len(h) < 3 {
		return nil, fmt.Errorf("error ReadAll: %v", err)
	}
	//
	meta := Meta{}
	err = yaml.Unmarshal(h, &meta)
	if err != nil {
		return nil, fmt.Errorf("error reading yml: %v", err)
	}
	parsedDate, err := time.Parse(dateFormat, meta.Date)
	if err != nil {
		//return nil, fmt.Errorf("error format date %s: %v", meta.Date, err)
	}
	meta.ParsedDate = parsedDate
	return &meta, nil
}

//func getMeta(path, dateFormat string) (*Meta, error) {
//	filePath := filepath.Join(path, "meta.yml")
//	metaraw, err := ioutil.ReadFile(filePath)
//	if err != nil {
//		return nil, fmt.Errorf("error while reading file %s: %v", filePath, err)
//	}
//	meta := Meta{}
//	err = yaml.Unmarshal(metaraw, &meta)
//	if err != nil {
//		return nil, fmt.Errorf("error reading yml in %s: %v", filePath, err)
//	}
//	parsedDate, err := time.Parse(dateFormat, meta.Date)
//	if err != nil {
//		return nil, fmt.Errorf("error parsing date in %s: %v", filePath, err)
//	}
//	meta.ParsedDate = parsedDate
//	return &meta, nil
//}

func getHTML(br *bufio.Reader) ([]byte, error) {
	input, _ := ioutil.ReadAll(br)
	html := blackfriday.MarkdownCommon(input)
	replaced, err := replaceCodeParts(html)
	if err != nil {
		return nil, fmt.Errorf("error during syntax highlighting : %v", err)
	}
	return []byte(replaced), nil
}

func getImages(path string) (string, []string, error) {
	dirPath := filepath.Join(path, "images")
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, nil
		}
		return "", nil, fmt.Errorf("error while reading folder %s: %v", dirPath, err)
	}
	images := []string{}
	for _, file := range files {
		images = append(images, file.Name())
	}
	return dirPath, images, nil
}

func replaceCodeParts(htmlFile []byte) (string, error) {
	byteReader := bytes.NewReader(htmlFile)
	doc, err := goquery.NewDocumentFromReader(byteReader)
	if err != nil {
		return "", fmt.Errorf("error while parsing html: %v", err)
	}
	// find code-parts via css selector and replace them with highlighted versions
	doc.Find("code[class*=\"language-\"]").Each(func(i int, s *goquery.Selection) {
		oldCode := s.Text()
		formatted, _ := syntaxhighlight.AsHTML([]byte(oldCode))
		s.SetHtml(string(formatted))
	})
	new, err := doc.Html()
	if err != nil {
		return "", fmt.Errorf("error while generating html: %v", err)
	}
	// replace unnecessarily added html tags
	new = strings.Replace(new, "<html><head></head><body>", "", 1)
	new = strings.Replace(new, "</body></html>", "", 1)
	return new, nil
}

func (p ByDateDesc) Len() int {
	return len(p)
}

func (p ByDateDesc) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func (p ByDateDesc) Less(i, j int) bool {
	return p[i].Meta.ParsedDate.After(p[j].Meta.ParsedDate)
}
