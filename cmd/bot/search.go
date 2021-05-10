package main

import (
	"os"
	"path/filepath"

	"github.com/blevesearch/bleve"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/gotd/getdoc"
	"github.com/gotd/tl"

	"github.com/gotd/bot/internal/docs"
)

func setupIndex(sessionDir, schemaPath string) (_ *docs.Search, rerr error) {
	f, err := os.Open(schemaPath)
	if err != nil {
		return nil, xerrors.Errorf("open: %w", err)
	}

	sch, err := tl.Parse(f)
	if err != nil {
		return nil, xerrors.Errorf("parse: %w", err)
	}

	index, err := bleve.New(filepath.Join(sessionDir, "docs.index"), bleve.NewIndexMapping())
	if err != nil {
		return nil, xerrors.Errorf("create indexer: %w", err)
	}
	defer func() {
		if rerr != nil {
			multierr.AppendInto(&rerr, index.Close())
		}
	}()

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
