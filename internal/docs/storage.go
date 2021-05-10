package docs

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/document"
	"golang.org/x/xerrors"

	"github.com/gotd/getdoc"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/tl"
)

func definitionType(d tl.Definition) string {
	if len(d.Namespace) == 0 {
		return d.Name
	}
	return fmt.Sprintf("%s.%s", strings.Join(d.Namespace, "."), d.Name)
}

// Search is a abstraction for searching docs.
type Search struct {
	idx     bleve.Index
	data    map[string]tl.SchemaDefinition
	docs    *getdoc.Doc
	goNames map[uint32]func() bin.Object
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

	mapping := indexer.Mapping()
	for _, def := range schema.Definitions {
		id := fmt.Sprintf("%x", def.Definition.ID)

		doc := document.NewDocument(id)
		doc.AddField(document.NewTextField("type_id", nil, []byte(id)))
		if err := mapping.MapDocument(doc, Alias(def)); err != nil {
			return nil, xerrors.Errorf("index %s: %w", id, err)
		}
		s.data[id] = def
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
			GoName:           getType(s.goNames[def.Definition.ID]()),
			NamespacedName:   typeKey,
			Constructor:      constructorDoc,
			Method:           methodDoc,
		})
	}
	return result, nil
}
