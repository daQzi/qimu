package objects

import "testing"

func TestReferenceStrictContract(t *testing.T) {
	good := `{"objectId":"brand-1","version":1,"type":"brand","schemaVersion":1}`
	if _, err := DecodeReference([]byte(good)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{good + " {}", `{"objectId":"x","version":101,"type":"brand","schemaVersion":1}`, `{"objectId":"x","version":1,"type":"product","schemaVersion":1}`, `{"objectId":"x","version":1,"type":"brand","schemaVersion":2}`, `{"objectId":"x","version":1,"type":"brand","schemaVersion":1,"userId":"other"}`, "null"} {
		if _, err := DecodeReference([]byte(raw)); err == nil {
			t.Fatal("accepted incompatible reference", raw)
		}
	}
}
