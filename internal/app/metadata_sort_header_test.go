package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMetadataSortHeaderReflectsEffectiveOrder(t *testing.T) {
	for _, tc := range []struct {
		column  metadataColumn
		heading string
	}{
		{metadataExtension, "↑ EXTENSION"}, {metadataSize, "↓ SIZE"}, {metadataModified, "↓ MODIFIED"}, {metadataCreated, "↓ CREATED"},
	} {
		p := panel{metadata: tc.column, sortByMetadata: true}
		if !p.metadataSortActive() || p.metadataSortHeading() != tc.heading {
			t.Fatal("incorrect metadata sort direction")
		}
		p.sortByMetadata = false
		if p.metadataSortActive() || p.metadataSortHeading() != tc.column.heading() {
			t.Fatal("disabled metadata sort retained an arrow")
		}
	}
	for _, p := range []panel{{metadata: metadataNone, sortByMetadata: true}, {metadata: metadataSize, sortByMetadata: true, drives: true}} {
		if p.metadataSortActive() {
			t.Fatal("inactive metadata sort hid the name-sort indicator")
		}
	}
}

func TestSortHeaderPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SORT")
	if dir == "" {
		t.Skip("optional sort header preview")
	}
	v := newPreviewView()
	v.panels[1].sortByMetadata = true
	v.panels[2].metadata, v.panels[2].sortByMetadata = metadataExtension, true
	for i := range v.panels {
		v.panels[i].resort()
	}
	for _, mode := range []string{"normal", "follow"} {
		v.followMe = mode == "follow"
		if err := renderViewPNG(filepath.Join(dir, "sort-"+mode+".png"), v); err != nil {
			t.Fatal(err)
		}
	}
}
