// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"io"
)

//go:embed template/topology.html
var topologyHTML string

//go:embed template/cytoscape.min.js
var cytoscapeJS string

//go:embed template/dagre.min.js
var dagreJS string

//go:embed template/cytoscape-dagre.min.js
var cytoscapeDagreJS string

type templateData struct {
	Namespace        string
	CytoscapeJS      template.JS
	DagreJS          template.JS
	CytoscapeDagreJS template.JS
	TopologyJSON     template.JS
}

func generateHTML(data *vizData, w io.Writer) error {
	tmpl, err := template.New("topology").Parse(topologyHTML)
	if err != nil {
		return err
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return tmpl.Execute(w, templateData{
		Namespace:        data.Namespace,
		CytoscapeJS:      template.JS(cytoscapeJS),
		DagreJS:          template.JS(dagreJS),
		CytoscapeDagreJS: template.JS(cytoscapeDagreJS),
		TopologyJSON:     template.JS(jsonBytes),
	})
}
