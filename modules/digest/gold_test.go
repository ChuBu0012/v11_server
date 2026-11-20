package digest

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

const assocFixture = `<td><span id="DetailPlace_uc_goldprices1_lblAsTime"><b><font color="#000000">20/11/2569 เวลา 17:29 น. (ครั้งที่ 90)</font></b></span></td>
<td><span id="DetailPlace_uc_goldprices1_lblBLSell"><b><font color="Red">57,600.00</font></b></span></td>
<td><span id="DetailPlace_uc_goldprices1_lblBLBuy"><b><font color="Red">57,400.00</font></b></span></td>
<td><span id="DetailPlace_uc_goldprices1_lblOMSell"><b><font color="Red">58,400.00</font></b></span></td>
<td><span id="DetailPlace_uc_goldprices1_lblOMBuy"><b><font color="Red">56,258.76</font></b></span></td>`

func TestParseAssociation(t *testing.T) {
	bar, barBuy, orn, ornBuy, at, ok := parseAssociation(assocFixture)
	if !ok || bar != 57600 || barBuy != 57400 || orn != 58400 || ornBuy != 56258.76 || at != "20/11/2569 เวลา 17:29 น. (ครั้งที่ 90)" {
		t.Fatalf("parsed = %v %v %v %v %q ok=%v", bar, barBuy, orn, ornBuy, at, ok)
	}
}

func TestParseAssociationMissing(t *testing.T) {
	if _, _, _, _, _, ok := parseAssociation("<html>redesigned</html>"); ok {
		t.Fatal("missing IDs must not parse")
	}
}

func TestParseSpotAndTHB(t *testing.T) {
	if v, ok := parseSpot([]byte(`{"xau":{"price":3541.15},"spot_usd_oz":3541.149902}`)); !ok || math.Abs(v-3541.15) > 0.01 {
		t.Fatalf("spot = %v ok=%v", v, ok)
	}
	if _, ok := parseSpot([]byte(`{}`)); ok {
		t.Fatal("empty spot must fail")
	}
	if v, ok := parseTHB([]byte(`{"result":"success","rates":{"THB":33.94469}}`)); !ok || v != 33.94469 {
		t.Fatalf("thb = %v ok=%v", v, ok)
	}
}

func TestEstimateBar(t *testing.T) {
	if v := estimateBar(3541.15, 33.94469); math.Abs(v-56850) > 100 {
		t.Fatalf("estimate = %.0f, want ≈56850", v)
	}
}

func TestFetchFallsBackToEstimate(t *testing.T) {
	assoc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer assoc.Close()
	spot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"spot_usd_oz":3541.15}`)) }))
	defer spot.Close()
	fx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"rates":{"THB":33.94469}}`)) }))
	defer fx.Close()
	f := &GoldFetcher{client: http.DefaultClient, assocURL: assoc.URL, spotURL: spot.URL, fxURL: fx.URL}
	q := f.Fetch(context.Background())
	if !q.Estimated || q.BarSell < 56000 || q.SpotUSD != 3541.15 {
		t.Fatalf("quote = %+v", q)
	}
	okAssoc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(assocFixture)) }))
	defer okAssoc.Close()
	f.assocURL = okAssoc.URL
	if q := f.Fetch(context.Background()); q.Estimated || q.BarSell != 57600 || q.OrnSell != 58400 {
		t.Fatalf("association quote = %+v", q)
	}
}
