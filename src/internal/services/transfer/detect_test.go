package transfer

import (
	"testing"
	"time"
)

func TestDetectTellsFilesApart(t *testing.T) {
	pack, err := Zip([]File{{"setup.json", []byte(`{"format":"authama-setup"}`)}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	other, _ := Zip([]File{{"hosts.csv", []byte("a,b\n")}}, time.Now())
	for name, c := range map[string]struct {
		data []byte
		want Kind
	}{
		"full":    {[]byte("age-encryption.org/v1\n-> scrypt abc 18\n"), KindFull},
		"bundle":  {[]byte("age-encryption.org/v1\n-> X25519 abc\n"), KindBundle},
		"setup":   {[]byte("\n {\"format\": \"authama-setup\", \"version\": 1}"), KindSetup},
		"json":    {[]byte(`{"hosts": []}`), KindUnknown},
		"pack":    {pack, KindPack},
		"pack2":   {other, KindPackOnly},
		"config":  {[]byte("# mine\nHost web\n  HostName web.example\n"), KindSSHConfig},
		"termius": {[]byte("Groups,Label,Tags,Hostname/IP,Protocol,Port\n"), KindTermius},
		"text":    {[]byte("hello"), KindUnknown},
	} {
		if got := Detect(c.data); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}

func TestCheckRefusesUnknownPartsAndKeepsOrder(t *testing.T) {
	ids, err := Check(Options{Include: []string{"setup,termius", "setup"}})
	if err != nil || len(ids) != 2 || ids[0] != "termius" || ids[1] != "setup" {
		t.Fatalf("%v %v", ids, err)
	}
	if _, err := Check(Options{Include: []string{"keys,private"}}); err == nil {
		t.Error("an unknown part was accepted")
	}
	if _, err := Check(Options{TermiusGroups: "nope"}); err == nil {
		t.Error("an unknown groups scheme was accepted")
	}
	if all, _ := Check(Options{Include: []string{"all"}}); len(all) != len(Parts) {
		t.Errorf("all: %v", all)
	}
}
