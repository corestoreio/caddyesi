package esitag_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/SchumacherFM/caddyesi/esitag"
	"github.com/corestoreio/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustOpenFile(file string) *os.File {
	f, err := os.Open(file)
	if err != nil {
		panic(fmt.Sprintf("%s => %s", file, err))
	}
	return f
}

func strReader(s string) io.ReadCloser {
	return ioutil.NopCloser(strings.NewReader(s))
}

func testRunner(fileOrContent string, wantTags esitag.Entities, wantErrBhf errors.BehaviourFunc) func(*testing.T) {
	var rc io.ReadCloser
	isFile := strings.HasSuffix(fileOrContent, ".html")
	if isFile {
		rc = mustOpenFile("testdata/" + fileOrContent)
		fc, err := ioutil.ReadFile("testdata/" + fileOrContent)
		if err != nil {
			panic(err)
		}
		fileOrContent = string(fc)
	} else {
		rc = strReader(fileOrContent)
	}

	return func(t *testing.T) {
		defer rc.Close()
		haveTags, err := esitag.Parse(rc)
		if wantErrBhf != nil {
			assert.Nil(t, haveTags)
			assert.True(t, wantErrBhf(err), "%+v", err)
			return
		}
		require.NoError(t, err)

		if have, want := len(haveTags), len(wantTags); have != want {
			t.Errorf("esitag.ESITags Count does not match: Have: %v Want: %v", have, want)
		}

		for i, tg := range wantTags {
			assert.Exactly(t, string(tg.RawTag), string(haveTags[i].RawTag), "RawTag Index %d", i)
			assert.Exactly(t, tg.DataTag.Start, haveTags[i].DataTag.Start, "Start Index %d", i)
			assert.Exactly(t, tg.DataTag.End, haveTags[i].DataTag.End, "End Index %d", i)
			if haveEnd, wantEnd := haveTags[i].DataTag.End, len(fileOrContent); haveEnd > wantEnd {
				t.Fatalf("For DataTag index %d the end %d is greater than the content length %d", i, haveEnd, wantEnd)
			}
			assert.Exactly(t, "<esi:"+string(tg.RawTag)+"/>", fileOrContent[haveTags[i].DataTag.Start:haveTags[i].DataTag.End], "Index %d", i)
		}
	}
}

// page3Results used in test and in benchmark; relates to file testdata/page3.html
var page3Results = esitag.Entities{
	&esitag.Entity{
		RawTag: []byte(`include src="https://micr1.service/customer/account" timeout="18ms" onerror="accountNotAvailable.html"`),
		DataTag: esitag.DataTag{
			Start: 2009,
			End:   2118,
		},
	},
	&esitag.Entity{
		RawTag: []byte(`include src="https://micr2.service/checkout/cart" timeout="19ms" onerror="nocart.html" forwardheaders="Cookie,Accept-Language,Authorization"`),
		DataTag: esitag.DataTag{
			Start: 4042,
			End:   4189,
		},
	},
	&esitag.Entity{
		RawTag: []byte("include src=\"https://micr3.service/page/lastviewed\" timeout=\"20ms\" onerror=\"nofooter.html\" forwardheaders=\"Cookie,Accept-Language,Authorization\""),
		DataTag: esitag.DataTag{
			Start: 4453,
			End:   4604,
		},
	},
}

func TestParseESITags_File(t *testing.T) {
	t.Run("Page0", testRunner(
		("page0.html"),
		esitag.Entities{
			&esitag.Entity{
				RawTag: []byte("include   src=\"https://micro.service/esi/foo\"\n                                            "),
				DataTag: esitag.DataTag{
					Start: 196,
					End:   293,
				},
			},
		},
		nil,
	))
	t.Run("Page1", testRunner(
		("page1.html"),
		esitag.Entities{
			&esitag.Entity{
				RawTag: []byte("include src=\"https://micro.service/esi/foo\" timeout=\"8ms\" onerror=\"mylocalFile.html\""),
				DataTag: esitag.DataTag{
					Start: 20644,
					End:   20735,
				},
			},
		},
		nil,
	))
	t.Run("Page2", testRunner(
		("page2.html"),
		esitag.Entities{
			&esitag.Entity{
				RawTag: []byte("include src=\"https://micro.service/customer/account\" timeout=\"8ms\" onerror=\"accountNotAvailable.html\""),
				DataTag: esitag.DataTag{
					Start: 6280,
					End:   6388,
				},
			},
			&esitag.Entity{
				RawTag: []byte(`include src="https://micro.service/checkout/cart" timeout="9ms" onerror="nocart.html" forwardheaders="Cookie,Accept-Language,Authorization"`),
				DataTag: esitag.DataTag{
					Start: 7103,
					End:   7249,
				},
			},
		},
		nil,
	))
	t.Run("Page3", testRunner(
		("page3.html"),
		page3Results,
		nil,
	))
}

var benchmarkParseESITags esitag.Entities

// BenchmarkParseESITags-4   	   50000	     32894 ns/op	 139.99 MB/s	    9392 B/op	      12 allocs/op
// BenchmarkParseESITags-4   	   50000	     31768 ns/op	 144.95 MB/s	    5137 B/op	       9 allocs/op <= sync.Pool Finder
// BenchmarkParseESITags-4   	   50000	     30989 ns/op	 148.60 MB/s	    1041 B/op	       8 allocs/op <= additional sync.Pool Scanner
// new features ...
// BenchmarkParseESITags-4         30000	     52291 ns/op	  88.06 MB/s	    3794 B/op	      44 allocs/op <= regex
// BenchmarkParseESITags-4         30000	     44564 ns/op	 103.33 MB/s	    2417 B/op	      22 allocs/op <= strings.FieldFunc
func BenchmarkParseESITags(b *testing.B) {
	f := mustOpenFile("testdata/page3.html")
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(fi.Size())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := f.Seek(0, 0); err != nil {
			b.Fatal(err)
		}
		var err error
		benchmarkParseESITags, err = esitag.Parse(f)
		if err != nil {
			b.Fatal(err)
		}

		if have, want := len(benchmarkParseESITags), len(page3Results); have != want {
			b.Fatalf("esitag.ESITags Count does not match: Have: %v Want: %v", have, want)
		}
	}
	for i, tg := range page3Results {
		if !bytes.Equal(tg.RawTag, benchmarkParseESITags[i].RawTag) {
			b.Errorf("Tag mismatch:want: %q\nhave: %q\n", tg.RawTag, benchmarkParseESITags[i].RawTag)
		}
	}
}

func TestParseESITags_String(t *testing.T) {
	t.Run("Five ESI Tags", testRunner(
		(`@<esi:include   src="https://micro1.service1/esi/foo"
                                            />@<esi:include   src="https://micro2.service2/esi/foo"
/>@<esi:include   src="https://micro3.service3/esi/foo"/>@<esi:include
src="https://micro4.service4/esi/foo"/>@<esi:include src="https://micro5.service5/esi/foo"/>@`),
		esitag.Entities{
			&esitag.Entity{
				RawTag: []byte("include   src=\"https://micro1.service1/esi/foo\"\n                                            "),
				DataTag: esitag.DataTag{
					Start: 1,
					End:   100,
				},
			},
			&esitag.Entity{
				RawTag: []byte("include   src=\"https://micro2.service2/esi/foo\"\n"),
				DataTag: esitag.DataTag{
					Start: 101,
					End:   156,
				},
			},
			&esitag.Entity{
				RawTag: []byte("include   src=\"https://micro3.service3/esi/foo\""),
				DataTag: esitag.DataTag{
					Start: 157,
					End:   211,
				},
			},
			&esitag.Entity{
				RawTag: []byte("include\nsrc=\"https://micro4.service4/esi/foo\""),
				DataTag: esitag.DataTag{
					Start: 212,
					End:   264,
				},
			},
			&esitag.Entity{
				RawTag: []byte("include src=\"https://micro5.service5/esi/foo\""),
				DataTag: esitag.DataTag{
					Start: 265,
					End:   317,
				},
			},
		},
		nil,
	))

	t.Run("Empty", testRunner(
		(``),
		nil,
		nil,
	))
	t.Run("Null Bytes", testRunner(
		("x \x00 <i>x</i>          \x00<esi:include\x00 src=\"https://...\" />\x00"),
		esitag.Entities{
			&esitag.Entity{
				RawTag: []byte("include\x00 src=\"https://...\" "),
				DataTag: esitag.DataTag{
					Start: 23,
					End:   57,
				},
			},
		},
		nil,
	))
	t.Run("Not supported scheme in src attribute", testRunner(
		("x \x00 <i>x</i>          \x00<esi:include\x00 src=\"ftp://...\" />\x00"),
		nil,
		errors.IsNotSupported,
	))
	t.Run("Missing EndTag, returns empty slice", testRunner(
		(`<esi:include src="..." <b>`),
		nil,
		nil,
	))
	t.Run("Error when parsing timeout attribute", testRunner(
		(`<esi:include src="gopher1" timeout="10xyz" />`),
		nil,
		errors.IsNotValid,
	))
	t.Run("Multitags in Buffer", testRunner(
		(`abcdefg<esi:include src="url1"/>u p<esi:include src="url2" />k`),
		esitag.Entities{
			&esitag.Entity{
				RawTag: []byte("include src=\"url1\""),
				DataTag: esitag.DataTag{
					Start: 7,
					End:   32,
				},
			},
			&esitag.Entity{
				RawTag: []byte("include src=\"url2\" "),
				DataTag: esitag.DataTag{
					Start: 35,
					End:   61,
				},
			},
		},
		nil,
	))
}