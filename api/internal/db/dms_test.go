package db

import (
	"testing"
	"time"

	"github.com/gocql/gocql"
)

func TestDeduplicateDMMetadataKeepsNewestRow(t *testing.T) {
	firstID, secondID := gocql.TimeUUID(), gocql.TimeUUID()
	newest := time.Unix(2, 0)
	metadata := deduplicateDMMetadata([]DMMetadata{
		{ID: firstID, LastMessageTime: newest},
		{ID: secondID, LastMessageTime: time.Unix(1, 0)},
		{ID: firstID, LastMessageTime: time.Unix(0, 0)},
	})

	if len(metadata) != 2 || metadata[0].ID != firstID || !metadata[0].LastMessageTime.Equal(newest) {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
}
