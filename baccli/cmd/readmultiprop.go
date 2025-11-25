// Copyright © 2017 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/spf13/viper"

	"github.com/spf13/cobra"

	"github.com/smart-core-os/gobacnet"
	"github.com/smart-core-os/gobacnet/property"
	"github.com/smart-core-os/gobacnet/types"
)

// readmultipropCmd represents the readmultiprop command
var readmultipropCmd = &cobra.Command{
	Use:   "multi",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: readMulti,
}

func readMulti(cmd *cobra.Command, args []string) {
	if listProperties {
		property.PrintAll()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	c, err := gobacnet.NewClient(viper.GetString("interface"), viper.GetInt("port"))

	// We need the actual address of the device first.
	resp, err := c.WhoIs(ctx, startRange, endRange)
	if err != nil {
		log.Fatal(err)
	}

	if len(resp) == 0 {
		log.Fatal("Device id was not found on the network.")
	}
	for _, d := range resp {
		dest := d

		rp := types.ReadPropertyData{
			Object: types.Object{
				ID: types.ObjectID{
					Type:     8,
					Instance: types.ObjectInstance(deviceID),
				},
				Properties: []types.Property{
					types.Property{
						ID:         property.ObjectList,
						ArrayIndex: gobacnet.ArrayAll,
					},
				},
			},
		}
		out, err := c.ReadProperty(ctx, dest, rp)
		if err != nil {
			log.Fatal(err)
			return
		}
		ids, ok := out.Object.Properties[0].Data.([]interface{})
		if !ok {
			fmt.Println("Unable to get object list")
			return
		}

		rpm := types.ReadMultipleProperty{}
		rpm.Objects = make([]types.Object, len(ids))
		for i, rawID := range ids {
			id, ok := rawID.(types.ObjectID)
			if !ok {
				log.Printf("Unable to read object id %v", rawID)
				return
			}
			rpm.Objects[i].ID = id

			rpm.Objects[i].Properties = []types.Property{
				types.Property{
					ID:         property.ObjectName,
					ArrayIndex: gobacnet.ArrayAll,
				},
				types.Property{
					ID:         property.Description,
					ArrayIndex: gobacnet.ArrayAll,
				},
			}
		}

		x, err := c.ReadMultiProperty(ctx, dest, rpm)
		if err != nil {
			log.Println(err)
		}
		fmt.Println(x)
	}
}

func init() {
	readpropCmd.AddCommand(readmultipropCmd)
	readmultipropCmd.Flags().IntVarP(&startRange, "start", "s", -1, "Start range of discovery")
	readmultipropCmd.Flags().IntVarP(&endRange, "end", "e", int(0xBAC0), "End range of discovery")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// readmultipropCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// readmultipropCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
