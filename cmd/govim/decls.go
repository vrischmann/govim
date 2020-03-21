package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/govim/govim"
	"github.com/govim/govim/cmd/govim/internal/golang_org_x_tools/lsp/protocol"
)

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

	tmp, err := v.server.DocumentSymbol(context.Background(), params)
	if err != nil {
		return fmt.Errorf("called to gopls.DocumentSymbol failed: %v", err)
	}
	if len(tmp) == 0 {
		return nil
	}

	var locs []string
	for _, genericSymbol := range tmp {
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

		locs = append(locs, fmt.Sprintf("%d\t%s %s", int(symbol.Location.Range.Start.Line), symbol.Kind, symbol.Name))
	}

	v.ChannelCall("GOVIM_internal_Decls", locs)
	return nil
}
