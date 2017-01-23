// Copyright 2016-2017, Cyrill @ Schumacher.fm and the CaddyESI Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not
// use this file except in compliance with the License. You may obtain a copy of
// the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations under
// the License.

package main

import (
	"fmt"

	"github.com/vdobler/ht/ht"
)

func init() {
	// For now we must create new pointers each time we want to run a test. A
	// single test cannot be shared between goroutines. This is a limitation
	// which can maybe fixed by a special handling of the Request and Jar field
	// in ht. This change might complicate things ...
	RegisterTest(page01(), page01(), page01())
}

var tc01 int // tc = test counter

func page01() (t *ht.Test) {
	tc01++
	t = &ht.Test{
		Name:        fmt.Sprintf("Page MS Cart Tiny Iteration %d", tc01),
		Description: `Request loads ms_cart_tiny.html from a micro service and embeds the checkout cart into its HTML`,
		Request:     makeRequestGET("page_cart_tiny.html"),
		Checks: makeChecklist200(
			&ht.Body{
				Contains: "demo-store.shop/autumn-pullie.html",
				Count:    2,
			},
			&ht.Body{
				Contains: ` class="page01CartLoaded"`,
				Count:    1,
			},
		),
	}
	return
}
