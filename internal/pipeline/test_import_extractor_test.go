package pipeline

import (
	"reflect"
	"sort"
	"testing"

	types "insightify/internal/types"
	"insightify/internal/wordidx"
)

// sortRanges sorts ranges by (FilePath, StartLine, EndLine) for stable comparison.
func sortRanges(in []types.ImportStatementRange) {
	sort.Slice(in, func(i, j int) bool {
		if in[i].FilePath != in[j].FilePath {
			return in[i].FilePath < in[j].FilePath
		}
		if in[i].StartLine != in[j].StartLine {
			return in[i].StartLine < in[j].StartLine
		}
		return in[i].EndLine < in[j].EndLine
	})
}

func TestImportExtractor_WithThresholdAndWeakerIdentifiers(t *testing.T) {
	type args struct {
		keywords  []wordidx.PosRef
		idents    []wordidx.PosRef
		kwMag     int
		idMag     int
		threshold int
	}
	tests := []struct {
		name string
		args args
		want []types.ImportStatementRange
	}{
		{
			name: "Single keyword peak + one identifier pushes wider region over threshold",
			// kwMag=4 gives keyword decay: ...7:1, 8:2, 9:3, 10:4, 11:3, 12:2, 13:1...
			// idMag=2 at line 12 adds: 12:+2, 11:+1, 13:+1
			// threshold=2 -> contiguous region should be [8..13]
			args: args{
				keywords:  []wordidx.PosRef{{FilePath: "a.go", Line: 10}},
				idents:    []wordidx.PosRef{{FilePath: "a.go", Line: 12}},
				kwMag:     4,
				idMag:     2,
				threshold: 2,
			},
			want: []types.ImportStatementRange{
				{FilePath: "a.go", StartLine: 8, EndLine: 13},
			},
		},
		{
			name: "Two separated keyword peaks; only near-peak lines cross a higher threshold",
			// kwMag=3 -> near 5: 3,2,1; near 20: 3,2,1
			// idMag=1 at 6 and 18 only add +1 at centers (decay to 0 next step)
			// threshold=3 -> ranges: [5..6] (5:3, 6:2+1) and [20..20] (20:3)
			args: args{
				keywords: []wordidx.PosRef{
					{FilePath: "a.go", Line: 5},
					{FilePath: "a.go", Line: 20},
				},
				idents: []wordidx.PosRef{
					{FilePath: "a.go", Line: 6},
					{FilePath: "a.go", Line: 18},
				},
				kwMag:     3,
				idMag:     1,
				threshold: 3,
			},
			want: []types.ImportStatementRange{
				{FilePath: "a.go", StartLine: 5, EndLine: 6},
				{FilePath: "a.go", StartLine: 20, EndLine: 20},
			},
		},
		{
			name: "Identifiers on both sides lift sub-threshold shoulders to meet threshold",
			// kwMag=2 around 100: 100:2, 99:1, 101:1
			// idMag=1 at 99 and 101 adds +1 to those lines only
			// threshold=2 -> [99..101]
			args: args{
				keywords:  []wordidx.PosRef{{FilePath: "b.go", Line: 100}},
				idents:    []wordidx.PosRef{{FilePath: "b.go", Line: 99}, {FilePath: "b.go", Line: 101}},
				kwMag:     2,
				idMag:     1,
				threshold: 2,
			},
			want: []types.ImportStatementRange{
				{FilePath: "b.go", StartLine: 99, EndLine: 101},
			},
		},
		{
			name: "Different files are processed independently",
			// File x.go has a peak at 10 (kwMag=3) -> >=2 spans [9..11]
			// File y.go has a peak at 30 (kwMag=3) -> >=2 spans [29..31]
			// No identifiers needed; idMag=1 still fine.
			args: args{
				keywords: []wordidx.PosRef{
					{FilePath: "x.go", Line: 10},
					{FilePath: "y.go", Line: 30},
				},
				idents:    nil,
				kwMag:     3,
				idMag:     1,
				threshold: 2,
			},
			want: []types.ImportStatementRange{
				{FilePath: "x.go", StartLine: 9, EndLine: 11},
				{FilePath: "y.go", StartLine: 29, EndLine: 31},
			},
		},
		{
			name: "Out-of-range or non-positive lines are ignored; returns empty",
			args: args{
				keywords:  []wordidx.PosRef{{FilePath: "z.go", Line: 0}, {FilePath: "z.go", Line: -5}},
				idents:    []wordidx.PosRef{{FilePath: "z.go", Line: 0}},
				kwMag:     4,
				idMag:     2,
				threshold: 2,
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		tt := tt // capture
		t.Run(tt.name, func(t *testing.T) {
			got := importStatementExtractorWithThreshold(
				tt.args.keywords,
				tt.args.idents,
				tt.args.kwMag,
				tt.args.idMag,
				tt.args.threshold,
			)
			sortRanges(got)
			sortRanges(tt.want)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("unexpected ranges.\n got: %+v\nwant: %+v", got, tt.want)
			}
		})
	}
}

func TestImportExtractor_DefaultWrapperBehavior(t *testing.T) {
	// Verifies that the thin wrapper uses:
	//   identifierMagnitude = max(1, magnitude/2)
	//   threshold           = max(1, magnitude/2)
	// With magnitude=4 -> idMag=2, threshold=2 (same as first test case idea).
	keywords := []wordidx.PosRef{{FilePath: "a.go", Line: 10}}
	idents := []wordidx.PosRef{{FilePath: "a.go", Line: 12}}
	got := importStatementExtractor(keywords, idents, 4)

	want := []types.ImportStatementRange{
		{FilePath: "a.go", StartLine: 8, EndLine: 13},
	}
	sortRanges(got)
	sortRanges(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default wrapper produced unexpected ranges.\n got: %+v\nwant: %+v", got, want)
	}
}
