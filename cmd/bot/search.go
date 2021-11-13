package main

import (
	"os"
	"path/filepath"

	"github.com/blevesearch/bleve/v2"
	"github.com/go-faster/errors"
	"go.uber.org/multierr"

	"github.com/gotd/getdoc"
	"github.com/gotd/tl"

	"github.com/gotd/bot/internal/docs"
)

func setupIndex(sessionDir, schemaPath string) (_ *docs.Search, rerr error) {
	f, err := os.Open(schemaPath)
	if err != nil {
		return nil, errors.Wrap(err, "open")
	}
	defer func() { _ = f.Close() }()

	sch, err := tl.Parse(f)
	if err != nil {
		return nil, errors.Wrap(err, "parse")
	}

	indexPath := filepath.Join(sessionDir, "docs.index")
	index, err := bleve.Open(indexPath)
	switch {
	case errors.Is(err, bleve.ErrorIndexPathDoesNotExist):
		index, err = bleve.New(indexPath, bleve.NewIndexMapping())
		if err != nil {
			return nil, errors.Wrap(err, "create indexer")
		}
	case err != nil:
		return nil, errors.Wrap(err, "open index")
	}
	defer func() {
		if rerr != nil {
			multierr.AppendInto(&rerr, index.Close())
		}
	}()

	doc, err := getdoc.Load(getdoc.LayerLatest)
	if err != nil {
		return nil, errors.Wrap(err, "load docs")
	}

	search, err := docs.IndexSchema(index, sch, doc)
	if err != nil {
		return nil, errors.Wrap(err, "index schema")
	}

	return search, nil
}
