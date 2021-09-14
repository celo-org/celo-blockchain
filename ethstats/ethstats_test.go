package ethstats

import (
	"strconv"
	"testing"
)

func TestParseEthstatsURL(t *testing.T) {
	cases := []struct {
		url              string
		node, pass, host string
	}{
		{
			url:  `"debug meowsbits"@ws://mordor.dash.fault.dev:3000`,
			node: "debug meowsbits", host: "ws://mordor.dash.fault.dev:3000",
		},
		{
			url:  `"debug @meowsbits"@ws://mordor.dash.fault.dev:3000`,
			node: "debug @meowsbits", host: "ws://mordor.dash.fault.dev:3000",
		},
		{
			url:  `"debug: @meowsbits"@ws://mordor.dash.fault.dev:3000`,
			node: "debug: @meowsbits", host: "ws://mordor.dash.fault.dev:3000",
		},
		{
			url:  `name@ws://mordor.dash.fault.dev:3000`,
			node: "name", host: "ws://mordor.dash.fault.dev:3000",
		},
		{
			url:  `name@ws://mordor.dash.fault.dev:3000`,
			node: "name", host: "ws://mordor.dash.fault.dev:3000",
		},
		{
			url:  `@ws://mordor.dash.fault.dev:3000`,
			node: "", host: "ws://mordor.dash.fault.dev:3000",
		},
		{
			url:  `@ws://mordor.dash.fault.dev:3000`,
			node: "", host: "ws://mordor.dash.fault.dev:3000",
		},
	}

	for i, c := range cases {
		var (
			name          string
			celostatsHost string
		)
		if err := parseEthstatsURL(c.url, &name, &celostatsHost); err != nil {
			t.Fatal(err)
		}

		// unquote because the value provided will be used as a CLI flag value, so unescaped quotes will be removed
		nodeUnquote, err := strconv.Unquote(name)
		if err == nil {
			name = nodeUnquote
		}

		if name != c.node {
			t.Errorf("case=%d mismatch node value, got: %v ,want: %v", i, name, c.node)
		}
		if celostatsHost != c.host {
			t.Errorf("case=%d mismatch host value, got: %v ,want: %v", i, celostatsHost, c.host)
		}
	}

}
