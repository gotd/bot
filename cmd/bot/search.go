package main

import (
	"os"

	"github.com/blevesearch/bleve"
	"golang.org/x/xerrors"

	"github.com/gotd/getdoc"
	"github.com/gotd/tl"

	"github.com/gotd/bot/internal/docs"
)

func setupSearch(p string) (*docs.Search, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, xerrors.Errorf("open: %w", err)
	}

	sch, err := tl.Parse(f)
	if err != nil {
		return nil, xerrors.Errorf("parse: %w", err)
	}

	index, err := bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		return nil, xerrors.Errorf("create indexer: %w", err)
	}

	doc, err := getdoc.Load(getdoc.LayerLatest)
	if err != nil {
		return nil, xerrors.Errorf("load docs: %w", err)
	}

	search, err := docs.IndexSchema(index, sch, doc)
	if err != nil {
		return nil, xerrors.Errorf("index schema: %w", err)
	}

	return search, nil
}
