package docs

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"golang.org/x/xerrors"

	"github.com/gotd/getdoc"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/tl"
)

func namespacedName(name string, namespace []string) string {
	if len(namespace) == 0 {
		return name
	}
	return fmt.Sprintf("%s.%s", strings.Join(namespace, "."), name)
}

func definitionType(d tl.Definition) string {
	return namespacedName(d.Name, d.Namespace)
}

// Search is a abstraction for searching docs.
type Search struct {
	idx     bleve.Index
	data    map[string]tl.SchemaDefinition
	docs    *getdoc.Doc
	goNames map[uint32]func() bin.Object
}

// Close closes underlying index.
func (s *Search) Close() error {
	return s.idx.Close()
}

// IndexSchema creates new Search object.
func IndexSchema(indexer bleve.Index, schema *tl.Schema, docs *getdoc.Doc) (*Search, error) {
	type Alias tl.SchemaDefinition

	s := &Search{
		idx:     indexer,
		data:    make(map[string]tl.SchemaDefinition, len(schema.Definitions)),
		docs:    docs,
		goNames: tg.TypesConstructorMap(),
	}

	for _, def := range schema.Definitions {
		id := fmt.Sprintf("%x", def.Definition.ID)

		doc, err := indexer.Document(id)
		if err != nil {
			return nil, xerrors.Errorf("try find %q: %w", id, err)
		}
		s.data[id] = def
		if doc != nil {
			continue
		}

		if err := indexer.Index(id, map[string]interface{}{
			"id":         id,
			"idx":        "0x" + id,
			"definition": Alias(def),
			"name":       def.Definition.Name,
			"namespace":  def.Definition.Namespace,
			"fullName":   definitionType(def.Definition),
			"goName":     s.goName(def.Definition.ID),
			"category":   def.Category.String(),
		}); err != nil {
			return nil, xerrors.Errorf("index %s: %w", id, err)
		}
	}

	return s, nil
}

type SearchResult struct {
	tl.SchemaDefinition
	NamespacedName string
	GoName         string
	Constructor    getdoc.Constructor
	Method         getdoc.Method
}

func getType(v interface{}) string {
	if t := reflect.TypeOf(v); t.Kind() == reflect.Ptr {
		return t.Elem().Name()
	} else {
		return t.Name()
	}
}

func (s *Search) goName(id uint32) string {
	return getType(s.goNames[id]())
}

// Match searches docs using given text query.
func (s *Search) Match(q string) ([]SearchResult, error) {
	query := bleve.NewQueryStringQuery(q)
	req := bleve.NewSearchRequest(query)
	searchResult, err := s.idx.Search(req)
	if err != nil {
		return nil, xerrors.Errorf("query index %q: %w", q, err)
	}

	result := make([]SearchResult, 0, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		def, ok := s.data[hit.ID]
		if !ok {
			return nil, xerrors.Errorf("%s not found", hit.ID)
		}

		typeKey := definitionType(def.Definition)
		constructorDoc := s.docs.Constructors[typeKey]
		methodDoc := s.docs.Methods[typeKey]

		result = append(result, SearchResult{
			SchemaDefinition: def,
			GoName:           s.goName(def.Definition.ID),
			NamespacedName:   typeKey,
			Constructor:      constructorDoc,
			Method:           methodDoc,
		})
	}
	return result, nil
}
