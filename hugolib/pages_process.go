// Copyright 2019 The Hugo Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hugolib

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gohugoio/hugo/config"
	"github.com/gohugoio/hugo/parser/pageparser"
	"github.com/gohugoio/hugo/source"

	"golang.org/x/sync/errgroup"

	"github.com/gohugoio/hugo/hugofs"
)

func newPagesProcessor(h *HugoSites, sp *source.SourceSpec) *pagesProcessor {
	s := h.Sites[0]
	return &pagesProcessor{
		m: s.pageMap,

		chanFile:   make(chan hugofs.FileMetaDirEntry, 10),
		chanLeaf:   make(chan hugofs.FileMetaDirEntry, 10),
		chanBranch: make(chan hugofs.FileMetaDirEntry, 10),
	}
}

type pagesProcessor struct {
	m *pageMap

	ctx context.Context

	chanFile   chan hugofs.FileMetaDirEntry
	chanBranch chan hugofs.FileMetaDirEntry
	chanLeaf   chan hugofs.FileMetaDirEntry

	itemGroup *errgroup.Group
}

type pageProcessFiType int

const (
	pageProcessFiTypeStaticFile pageProcessFiType = iota
	pageProcessFiTypeLeaf
	pageProcessFiTypeBranch
)

func (p *pagesProcessor) Process(fi hugofs.FileMetaDirEntry, tp pageProcessFiType) error {
	if fi.IsDir() {
		return nil
	}

	var ch chan hugofs.FileMetaDirEntry
	switch tp {
	case pageProcessFiTypeLeaf:
		ch = p.chanLeaf
	case pageProcessFiTypeBranch:
		ch = p.chanBranch
	case pageProcessFiTypeStaticFile:
		ch = p.chanFile
	}

	select {
	case <-p.ctx.Done():
		return nil
	case ch <- fi:

	}

	return nil

}

func (p *pagesProcessor) copyFile(fim hugofs.FileMetaDirEntry) error {
	meta := fim.Meta()
	f, err := meta.Open()
	if err != nil {
		return fmt.Errorf("copyFile: failed to open: %w", err)
	}
	defer f.Close()

	s := p.m.s

	target := filepath.Join(s.PathSpec.GetTargetLanguageBasePath(), filepath.FromSlash(meta.PathInfo.Path()))

	fs := s.PublishFsStatic

	return s.publish(&s.PathSpec.ProcessingStats.Files, target, f, fs)
}

func (p *pagesProcessor) Start(ctx context.Context) context.Context {
	p.itemGroup, ctx = errgroup.WithContext(ctx)
	p.ctx = ctx
	numWorkers := config.GetNumWorkerMultiplier()
	if numWorkers > 1 {
		numWorkers = numWorkers / 2
	}

	for i := 0; i < numWorkers; i++ {
		p.itemGroup.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return nil
				case fi, ok := <-p.chanLeaf:
					if !ok {
						return nil
					}
					if err := p.m.AddFi(fi, false); err != nil {
						if errors.Is(err, pageparser.ErrPlainHTMLDocumentsNotSupported) {
							// Reclassify this as a static file.
							if err := p.copyFile(fi); err != nil {
								return err
							}
							continue

						}
						return err
					}
				}
			}
		})

		p.itemGroup.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return nil
				case fi, ok := <-p.chanBranch:
					if !ok {
						return nil
					}
					if err := p.m.AddFi(fi, true); err != nil {
						return err
					}
				}
			}
		})

	}

	p.itemGroup.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case fi, ok := <-p.chanFile:
				if !ok {
					return nil
				}
				if err := p.copyFile(fi); err != nil {
					return err
				}
			}
		}

	})

	return ctx
}

func (p *pagesProcessor) Wait() error {
	close(p.chanLeaf)
	close(p.chanBranch)
	close(p.chanFile)
	return p.itemGroup.Wait()
}
