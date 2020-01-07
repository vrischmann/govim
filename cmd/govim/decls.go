package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/govim/govim"
	"github.com/govim/govim/cmd/govim/internal/golang_org_x_tools/lsp/protocol"
)

func appendDocumentSymbolsToQuickfix(filename string, locs []quickfixEntry, symbols []protocol.SymbolInformation) []quickfixEntry {
	for _, symbol := range symbols {
		locs = append(locs, quickfixEntry{
			Filename: filename,
			Lnum:     int(symbol.Location.Range.Start.Line) + 1,
			Col:      int(symbol.Location.Range.Start.Character) + 1,
			Text:     fmt.Sprintf("%s %s", symbol.Kind, symbol.Name),
		})
	}
	return locs
}

func (v *vimstate) decls(flags govim.CommandFlags, args ...string) error {
	v.quickfixIsDiagnostics = false
	b, _, err := v.cursorPos()
	if err != nil {
		return fmt.Errorf("failed to get current position: %v", err)
	}

	params := &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: protocol.DocumentURI(b.URI()),
		},
	}

	// must be non-nil
	locs := []quickfixEntry{}

	tmp, err := v.server.DocumentSymbol(context.Background(), params)
	if err != nil {
		return fmt.Errorf("called to gopls.DocumentSymbol failed: %v", err)
	}
	if len(tmp) == 0 {
		return nil
	}

	symbols := make([]protocol.SymbolInformation, len(tmp))
	for i, genericSymbol := range tmp {
		s, ok := genericSymbol.(map[string]interface{})
		if !ok {
			continue
		}

		bytes, err := json.Marshal(s)
		if err != nil {
			return err
		}

		var symbol protocol.SymbolInformation
		if err := json.Unmarshal(bytes, &symbol); err != nil {
			return err
		}

		symbols[i] = symbol
	}

	locs = appendDocumentSymbolsToQuickfix(b.URI().Filename(), locs, symbols)

	toSort := locs
	sort.Slice(toSort, func(i, j int) bool {
		lhs, rhs := toSort[i], toSort[j]

		cmp := lhs.Lnum - rhs.Lnum
		if cmp == 0 {
			cmp = lhs.Col - rhs.Col
		}
		return cmp < 0
	})
	v.ChannelCall("setqflist", locs, "r")
	v.ChannelEx("copen")
	return nil
}
