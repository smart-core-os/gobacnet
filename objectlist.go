package gobacnet

import (
	"context"
	"fmt"

	"github.com/smart-core-os/gobacnet/property"
	bactype "github.com/smart-core-os/gobacnet/types"
)

func (c *Client) objectListLen(ctx context.Context, dev bactype.Device) (int, error) {
	rp := bactype.ReadPropertyData{
		Object: bactype.Object{
			ID: dev.ID,
			Properties: []bactype.Property{
				bactype.Property{
					ID:         property.ObjectList,
					ArrayIndex: 0,
				},
			},
		},
	}

	resp, err := c.ReadProperty(ctx, dev, rp)
	if err != nil {
		return 0, fmt.Errorf("reading property failed: %w", err)
	}

	if len(resp.Object.Properties) == 0 {
		return 0, fmt.Errorf("no data was returned")
	}

	data, ok := resp.Object.Properties[0].Data.(uint32)
	if !ok {
		return 0, fmt.Errorf("unable to get object length")
	}
	return int(data), nil
}

func (c *Client) objectsRange(ctx context.Context, dev bactype.Device, start, end int) ([]bactype.Object, error) {
	rpm := bactype.ReadMultipleProperty{
		Objects: []bactype.Object{
			bactype.Object{
				ID: dev.ID,
			},
		},
	}

	for i := start; i <= end; i++ {
		rpm.Objects[0].Properties = append(rpm.Objects[0].Properties, bactype.Property{
			ID:         property.ObjectList,
			ArrayIndex: uint32(i),
		})
	}
	resp, err := c.ReadMultiProperty(ctx, dev, rpm)
	if err != nil {
		return nil, fmt.Errorf("unable to read multiple properties: %w", err)
	}
	if len(resp.Objects) == 0 {
		return nil, fmt.Errorf("no data was returned")
	}

	objs := make([]bactype.Object, len(resp.Objects[0].Properties))

	for i, prop := range resp.Objects[0].Properties {
		id, ok := prop.Data.(bactype.ObjectID)
		if !ok {
			return nil, fmt.Errorf("expected type Object ID, got %T", prop.Data)
		}
		objs[i].ID = id
	}

	return objs, nil
}

const readPropRequestSize = 20

func objectCopy(dest bactype.ObjectMap, src []bactype.Object) {
	for _, o := range src {
		if dest[o.ID.Type] == nil {
			dest[o.ID.Type] = make(map[bactype.ObjectInstance]bactype.Object)
		}
		dest[o.ID.Type][o.ID.Instance] = o
	}

}

func (c *Client) objectList(ctx context.Context, dev *bactype.Device) error {
	dev.Objects = make(bactype.ObjectMap)

	l, err := c.objectListLen(ctx, *dev)
	if err != nil {
		return fmt.Errorf("unable to get list length: %w", err)
	}

	// Scan size is broken
	scanSize := int(dev.MaxApdu) / readPropRequestSize
	i := 0
	for i = 0; i < l/scanSize; i++ {
		start := i*scanSize + 1
		end := (i + 1) * scanSize

		objs, err := c.objectsRange(ctx, *dev, start, end)
		if err != nil {
			return fmt.Errorf("unable to retrieve objects between %d and %d: %w", start, end, err)
		}
		objectCopy(dev.Objects, objs)
	}
	start := i*scanSize + 1
	end := l
	if start <= end {
		objs, err := c.objectsRange(ctx, *dev, start, end)
		if err != nil {
			return fmt.Errorf("unable to retrieve objects between %d and %d: %w", start, end, err)
		}
		objectCopy(dev.Objects, objs)
	}
	return nil
}

func (c *Client) objectInformation(ctx context.Context, dev *bactype.Device, objs []bactype.Object) error {
	// Often times the map will re arrange the order it spits out
	// so we need to keep track since the response will be in the
	// same order we issue the commands.
	keys := make([]bactype.ObjectID, len(objs))
	counter := 0
	rpm := bactype.ReadMultipleProperty{
		Objects: []bactype.Object{},
	}

	for _, o := range objs {
		if o.ID.Type > maxStandardBacnetType {
			continue
		}
		keys[counter] = o.ID
		counter++
		rpm.Objects = append(rpm.Objects, bactype.Object{
			ID: o.ID,
			Properties: []bactype.Property{
				bactype.Property{
					ID:         property.ObjectName,
					ArrayIndex: bactype.ArrayAll,
				},
				bactype.Property{
					ID:         property.Description,
					ArrayIndex: bactype.ArrayAll,
				},
			},
		})

	}
	resp, err := c.ReadMultiProperty(ctx, *dev, rpm)
	if err != nil {
		return fmt.Errorf("unable to read multiple property: %w", err)
	}
	var name, description string
	var ok bool
	for i, r := range resp.Objects {
		name, ok = r.Properties[0].Data.(string)
		if !ok {
			return fmt.Errorf("expecting string got %T", r.Properties[0].Data)
		}
		description, ok = r.Properties[1].Data.(string)
		if !ok {
			return fmt.Errorf("expecting string got %T", r.Properties[1].Data)
		}
		obj := dev.Objects[keys[i].Type][keys[i].Instance]
		obj.Name = name
		obj.Description = description
		dev.Objects[keys[i].Type][keys[i].Instance] = obj
	}
	return nil
}

func (c *Client) allObjectInformation(ctx context.Context, dev *bactype.Device) error {
	objs := dev.ObjectSlice()
	incrSize := 5

	var err error
	for i := 0; i < len(objs); i += incrSize {
		subset := objs[i:min(i+incrSize, len(objs))]
		err = c.objectInformation(ctx, dev, subset)
		if err != nil {
			return err
		}
	}

	return nil
}

// Objects retrieves all of the objects within the given device and returns a
// device with these objects. Along with the list of objects, it will also
// gather additional information from the object such as the name and
// description of the objects. The device returned contains all of the name and
// description fields for all objects
func (c *Client) Objects(ctx context.Context, dev bactype.Device) (bactype.Device, error) {
	err := c.objectList(ctx, &dev)
	if err != nil {
		return dev, fmt.Errorf("unable to get object list: %w", err)
	}
	err = c.allObjectInformation(ctx, &dev)
	if err != nil {
		return dev, fmt.Errorf("unable to get object's information: %w", err)
	}
	return dev, nil
}
