package core

import (
	"context"
	"reflect"
	"testing"
)

func TestSplitOptionName(t *testing.T) {
	for name, want := range map[string][]string{
		"services.openssh.enable":                    {"services", "openssh", "enable"},
		`services.nginx.virtualHosts."a.b.lan".root`: {"services", "nginx", "virtualHosts", "a.b.lan", "root"},
		`users.users."pablo".openssh`:                {"users", "users", "pablo", "openssh"},
	} {
		got, err := SplitOptionName(name)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v %v", name, got, err)
		}
	}
	for _, bad := range []string{"", "a..b", ".a", "a.", `a."b`, "services.<name>.enable", "a.b c"} {
		if _, err := SplitOptionName(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestOptionValuesAcrossTheFleet(t *testing.T) {
	ctx := context.Background()
	s := fleetService(t, "testdata/fleet")
	names := []string{"services.openssh.settings.PermitRootLogin", "time.timeZone"}
	res, job, err := s.Jobs.OptionValues(ctx, names, nil)
	if err != nil || res != nil || job == nil {
		t.Fatalf("first request: %v %v %v", res, job, err)
	}
	wait(t, job)
	res, job, err = s.Jobs.OptionValues(ctx, names, nil)
	if err != nil || job != nil || len(res) != 2 {
		t.Fatalf("cached result: %v %v %v", res, job, err)
	}
	root := res[0]
	if root.Name != "services.openssh.settings.PermitRootLogin" || len(root.Hosts) != 4 {
		t.Fatalf("PermitRootLogin across the fleet: %+v", root)
	}
	by := map[string]HostValue{}
	for _, h := range root.Hosts {
		by[h.Host] = h
	}
	if by["pi"].Value != "yes" || !by["pi"].Exists || len(by["pi"].Files) == 0 || by["homeserver"].Value != "no" {
		t.Fatalf("values per machine: %+v", by)
	}
	if by["oldbox"].Error == "" {
		t.Fatal("a machine that does not evaluate must carry its error")
	}
	one, job, _ := s.Jobs.OptionValues(ctx, []string{"time.timeZone"}, []string{"pi"})
	if one == nil {
		wait(t, job)
		one, _, _ = s.Jobs.OptionValues(ctx, []string{"time.timeZone"}, []string{"pi"})
	}
	if len(one) != 1 || len(one[0].Hosts) != 1 || one[0].Hosts[0].Value != "Europe/Madrid" {
		t.Fatalf("one machine: %+v", one)
	}
	_, _, err = s.Jobs.OptionValues(ctx, []string{"services.<name>.enable"}, nil)
	isErr(t, err, ErrInvalid)
	_, _, err = s.Jobs.OptionValues(ctx, nil, nil)
	isErr(t, err, ErrInvalid)
}
