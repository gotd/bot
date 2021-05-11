package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/blevesearch/bleve"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/gotd/getdoc"
	"github.com/gotd/tl"

	"github.com/gotd/bot/internal/docs"
)

func parseSchema(schemaPath string) (_ *tl.Schema, rerr error) {
	f, err := os.Open(schemaPath)
	if err != nil {
		return nil, xerrors.Errorf("open: %w", err)
	}
	defer func() {
		multierr.AppendInto(&rerr, f.Close())
	}()

	sch, err := tl.Parse(f)
	if err != nil {
		return nil, xerrors.Errorf("parse: %w", err)
	}

	return sch, nil
}

func setupIndex(sessionDir, schemaPath string) (_ *docs.Search, rerr error) {
	sch, err := parseSchema(schemaPath)
	if err != nil {
		return nil, xerrors.Errorf("parse schema: %w", err)
	}

	indexPath := filepath.Join(sessionDir, "docs.index")
	index, err := bleve.Open(indexPath)
	switch {
	case errors.Is(err, bleve.ErrorIndexPathDoesNotExist):
		index, err = bleve.New(indexPath, bleve.NewIndexMapping())
		if err != nil {
			return nil, xerrors.Errorf("create indexer: %w", err)
		}
	case err != nil:
		return nil, xerrors.Errorf("open index: %w", err)
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
