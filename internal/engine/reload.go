package engine

import (
	"context"
	"os"
	"time"

	"example.com/cx/internal/config"
	"example.com/cx/internal/embed"
	"example.com/cx/internal/index"
	"example.com/cx/internal/ir"
	"example.com/cx/internal/java"
	"example.com/cx/internal/scan"
	"example.com/cx/internal/spring"
)

// reindex rebuilds lexical indexes from disk and bumps generation.
func (e *Engine) reindex() {
	res, err := scan.Scan(e.root, scan.Options{Excludes: e.excludes})
	if err != nil {
		e.log.Printf("reindex scan failed: %v", err)
		return
	}
	ext := java.NewExtractor()
	defer ext.Close()
	springExt := spring.NewExtractor()
	defer springExt.Close()
	bindExt := config.NewBindingExtractor()
	defer bindExt.Close()

	var facts []ir.FileFacts
	var springFacts []spring.Facts
	var cfgFiles []config.FileFacts
	var bindings []config.Binding
	var textLines []index.TextLine
	var chunks []embed.Chunk
	for _, f := range res.Files {
		data, err := os.ReadFile(f.AbsPath)
		if err != nil {
			continue
		}
		if f.IsJava {
			ff, err := ext.Extract(f.RelPath, data)
			if err != nil {
				continue
			}
			facts = append(facts, ff)
			chunks = append(chunks, embed.ChunksFromFacts(ff, data)...)
			springFacts = append(springFacts, springExt.Extract(f.RelPath, data))
			bindings = append(bindings, bindExt.ExtractBindings(f.RelPath, data)...)
			continue
		}
		if cfg, err := config.ParseIfConfig(f.RelPath, data); err == nil && cfg != nil {
			cfgFiles = append(cfgFiles, *cfg)
		}
		textLines = append(textLines, splitLinesRedacted(f.RelPath, data)...)
	}

	e.mu.Lock()
	e.files = res.Files
	e.stats = res.Stats
	e.gen++
	e.syms = index.Build(facts)
	e.text = index.BuildText(textLines)
	e.spring = spring.Build(springFacts)
	e.cfg = config.Build(cfgFiles, bindings)
	e.chunks = chunks
	semanticOn := e.prov != nil
	e.mu.Unlock()

	if semanticOn {
		e.setSemantic(ir.SemanticBuilding)
		go e.rebuildSemantic(chunks)
	}
}

func (e *Engine) rebuildSemantic(chunks []embed.Chunk) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := e.prov.Ping(ctx); err != nil {
		e.log.Printf("embedding provider unavailable: %v", err)
		e.setSemantic(ir.SemanticUnavailable)
		return
	}
	store := &embed.Store{}
	const batch = 8
	for i := 0; i < len(chunks); i += batch {
		if ctx.Err() != nil {
			e.setSemantic(ir.SemanticUnavailable)
			return
		}
		end := i + batch
		if end > len(chunks) {
			end = len(chunks)
		}
		part := chunks[i:end]
		inputs := make([]string, len(part))
		for j, c := range part {
			inputs[j] = c.Text
		}
		vecs, err := e.prov.Embed(ctx, inputs)
		if err != nil {
			e.log.Printf("embed batch failed: %v", err)
			e.setSemantic(ir.SemanticUnavailable)
			return
		}
		for j, v := range vecs {
			store.Add(part[j], v)
		}
	}
	e.mu.Lock()
	e.store = store
	e.chunks = chunks
	e.mu.Unlock()
	e.setSemantic(ir.SemanticReady)
	e.log.Printf("semantic index ready: %d chunks (gen %d)", store.Len(), e.Status().SemanticGeneration)
}
