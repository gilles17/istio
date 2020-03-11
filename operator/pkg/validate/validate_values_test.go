// Copyright 2019 Istio Authors
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

package validate

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/ghodss/yaml"

	"istio.io/istio/operator/pkg/helm"
	"istio.io/istio/operator/pkg/manifest"
	"istio.io/istio/operator/pkg/util"
	"istio.io/istio/pkg/test/env"
)

var (
	repoRootDir string
)

func init() {
	repoRootDir = env.IstioSrc
}

func TestValidateValues(t *testing.T) {
	tests := []struct {
		desc     string
		yamlStr  string
		wantErrs util.Errors
	}{
		{
			desc: "nil success",
		},
		{
			desc: "StarIPRange",
			yamlStr: `
global:
  proxy:
    includeIPRanges: "*"
    excludeIPRanges: "*"
`,
		},
		{
			desc: "ProxyConfig",
			yamlStr: `
global:
  podDNSSearchNamespaces:
  - "my-namespace"
  proxy:
    enabled: true
    includeIPRanges: "1.1.0.0/16,2.2.0.0/16"
    excludeIPRanges: "3.3.0.0/16,4.4.0.0/16"
    includeInboundPorts: "111,222"
    excludeInboundPorts: "333,444"
    clusterDomain: "my.domain"
    connectTimeout: "11s"
    drainDuration: "22s"
    parentShutdownDuration: "33s"
    concurrency: 5
`,
		},
		{
			desc: "CNIConfig",
			yamlStr: `
cni:
  cniBinDir: "/var/lib/cni/bin"
  cniConfDir: "/var/run/multus/cni/net.d"
`,
		},

		{
			desc: "BadIPRange",
			yamlStr: `
global:
  proxy:
    includeIPRanges: "1.1.0.256/16,2.2.0.257/16"
    excludeIPRanges: "3.3.0.0/33,4.4.0.0/34"
`,
			wantErrs: makeErrors([]string{`global.proxy.excludeIPRanges invalid CIDR address: 3.3.0.0/33`,
				`global.proxy.excludeIPRanges invalid CIDR address: 4.4.0.0/34`,
				`global.proxy.includeIPRanges invalid CIDR address: 1.1.0.256/16`,
				`global.proxy.includeIPRanges invalid CIDR address: 2.2.0.257/16`}),
		},
		{
			desc: "BadIPMalformed",
			yamlStr: `
global:
  proxy:
    includeIPRanges: "1.2.3/16,1.2.3.x/16"
`,
			wantErrs: makeErrors([]string{`global.proxy.includeIPRanges invalid CIDR address: 1.2.3/16`,
				`global.proxy.includeIPRanges invalid CIDR address: 1.2.3.x/16`}),
		},
		{
			desc: "BadIPWithStar",
			yamlStr: `
global:
  proxy:
    includeIPRanges: "*,1.1.0.0/16,2.2.0.0/16"
`,
			wantErrs: makeErrors([]string{`global.proxy.includeIPRanges invalid CIDR address: *`}),
		},
		{
			desc: "BadPortRange",
			yamlStr: `
global:
  proxy:
    includeInboundPorts: "111,65536"
    excludeInboundPorts: "-1,444"
`,
			wantErrs: makeErrors([]string{`value global.proxy.excludeInboundPorts:-1 falls outside range [0, 65535]`,
				`value global.proxy.includeInboundPorts:65536 falls outside range [0, 65535]`}),
		},
		{
			desc: "BadPortMalformed",
			yamlStr: `
global:
  proxy:
    includeInboundPorts: "111,222x"
`,
			wantErrs: makeErrors([]string{`global.proxy.includeInboundPorts : strconv.ParseInt: parsing "222x": invalid syntax`}),
		},
		{
			desc: "unknown field",
			yamlStr: `
global:
  proxy:
    foo: "bar"
`,
			wantErrs: makeErrors([]string{`unknown field "foo" in v1alpha1.ProxyConfig`}),
		},
		{
			desc: "unknown field",
			yamlStr: `
cni:
  foo: "bar"
`,
			wantErrs: makeErrors([]string{`unknown field "foo" in v1alpha1.CNIConfig`}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			root := make(map[string]interface{})
			err := yaml.Unmarshal([]byte(tt.yamlStr), &root)
			if err != nil {
				t.Fatalf("yaml.Unmarshal(%s): got error %s", tt.desc, err)
			}
			errs := CheckValues(root)
			if gotErr, wantErr := errs, tt.wantErrs; !util.EqualErrors(gotErr, wantErr) {
				t.Errorf("CheckValues(%s)(%v): gotErr:%s, wantErr:%s", tt.desc, tt.yamlStr, gotErr, wantErr)
			}
		})
	}
}

func TestValidateValuesFromProfile(t *testing.T) {
	t.Skip("https://github.com/istio/istio/issues/20112")
	// TODO port to new api
	tests := []struct {
		desc     string
		profile  string
		wantErrs util.Errors
	}{
		{
			profile: "default",
		},
		{
			profile: "demo",
		},
		{
			profile: "minimal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			pf, err := helm.ReadProfileYAML(tt.profile)
			if err != nil {
				t.Fatalf("fail to read profile: %s", tt.profile)
			}
			val, _, err := manifest.ParseK8SYAMLToIstioOperator(pf)
			if err != nil {
				t.Fatalf(" fail to parse profile to ISCP: (%s), got error %s", tt.profile, err)
			}
			errs := CheckValues(val.Spec.Values)
			if gotErr, wantErr := errs, tt.wantErrs; !util.EqualErrors(gotErr, wantErr) {
				t.Errorf("CheckValues of (%v): gotErr:%s, wantErr:%s", tt.profile, gotErr, wantErr)
			}
		})
	}
}
func TestValidateValuesFromValuesYAMLs(t *testing.T) {
	valuesYAML := ""
	var allFiles []string
	manifestDir := filepath.Join(repoRootDir, "manifests")
	for _, sd := range []string{"base", "gateways", "istio-cni", "istiocoredns", "istio-telemetry", "istio-control", "istio-policy"} {
		dir := filepath.Join(manifestDir, sd)
		files, err := util.FindFiles(dir, yamlFileFilter)
		if err != nil {
			t.Fatalf(err.Error())
		}
		allFiles = append(allFiles, files...)
	}
	allFiles = append(allFiles, filepath.Join(manifestDir, "global.yaml"))
	for _, f := range allFiles {
		b, err := ioutil.ReadFile(f)
		if err != nil {
			t.Fatal(err.Error())
		}
		valuesYAML, err = util.OverlayYAML(valuesYAML, string(b))
		if err != nil {
			t.Fatal(err.Error())
		}
		valuesTree := make(map[string]interface{})
		if err := yaml.Unmarshal([]byte(valuesYAML), &valuesTree); err != nil {
			t.Fatal(err.Error())
		}
		if err := CheckValues(valuesTree); err != nil {
			t.Fatalf("file %s failed validation with: %s", f, err)
		}
	}
}

func makeErrors(estr []string) util.Errors {
	var errs util.Errors
	for _, s := range estr {
		errs = util.AppendErr(errs, fmt.Errorf("%s", s))
	}
	return errs
}

func yamlFileFilter(path string) bool {
	return filepath.Base(path) == "values.yaml"
}
