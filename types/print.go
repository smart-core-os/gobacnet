package types

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/smart-core-os/gobacnet/property"
)

const defaultSpacing = 4

// String returns a pretty print of the ObjectID structure
func (id ObjectID) String() string {
	return fmt.Sprintf("Instance: %d Type: %s", id.Instance, id.Type.String())
}

// String returns a pretty print of the read multiple property structure
func (rp ReadMultipleProperty) String() string {
	buff := bytes.Buffer{}
	spacing := strings.Repeat(" ", defaultSpacing)
	for _, obj := range rp.Objects {
		buff.WriteString(obj.ID.String())
		buff.WriteString("\n")
		for _, prop := range obj.Properties {
			buff.WriteString(spacing)
			buff.WriteString(property.String(prop.ID))
			buff.WriteString(fmt.Sprintf("[%v]", prop.ArrayIndex))
			buff.WriteString(": ")
			buff.WriteString(fmt.Sprintf("%v", prop.Data))
			buff.WriteString("\n")
		}
		buff.WriteString("\n")
	}
	return buff.String()
}
