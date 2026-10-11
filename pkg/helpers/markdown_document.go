package helpers

import (
	"embed"
	"fmt"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbiw-widgets/markdown"
	"github.com/movsb/fbiw/input/sticks"
)

//go:embed markdown_document.html
var markdownDocumentAssets embed.FS

type markdownDocument struct {
	doc      *fbiw.Document
	viewport *fbiw.Scroll `css:"#viewport"`
	content  fbiw.Box     `css:"#content"`
}

// OpenMarkdownDocument opens a full-safe-area Markdown popup. The caller owns
// any surrounding UI state, such as overlay button hints.
func OpenMarkdownDocument(app *fbiw.App, opener *fbiw.Document, source string) (*fbiw.Document, error) {
	w := &markdownDocument{doc: app.NewPopup(markdownDocumentAssets, "markdown_document.html", opener)}
	w.doc.Bind(w)

	tree, err := markdown.Render(w.doc, []byte(source))
	if err != nil {
		w.doc.Close()
		return nil, fmt.Errorf("render Markdown: %w", err)
	}
	w.content.Base().AppendChild(tree)
	w.viewport.Activate()
	w.doc.Listen(fbiw.InputDownEvent, func(event *fbiw.Event) {
		if event.Input.Name == sticks.B {
			event.StopPropagation()
			w.doc.Close()
		}
	})
	return w.doc, nil
}
