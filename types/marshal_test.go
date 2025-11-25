package types

import (
	"encoding/json"
	"github.com/smart-core-os/gobacnet/types/objecttype"
	"reflect"
	"testing"
)

// TestMarshal tests encoding and decoding of the objectmap type. There is
// custom logic in it so we want to make sure it works.
func TestMarshal(t *testing.T) {
	test := ObjectMap{
		objecttype.AnalogInput:  make(map[ObjectInstance]Object),
		objecttype.BinaryOutput: make(map[ObjectInstance]Object),
	}
	test[objecttype.AnalogInput][0] = Object{Name: "Pizza Sensor"}
	test[objecttype.BinaryOutput][4] = Object{Name: "Should I Eat Pizza Sensor"}
	b, err := json.Marshal(test)
	if err != nil {
		t.Fatal(err)
	}

	out := make(ObjectMap, 0)
	err = json.Unmarshal(b, &out)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(test, out) {
		t.Fatalf("Encoding/decoding Object map is not equal. want %+v, got %+v", test, out)
	}

}
