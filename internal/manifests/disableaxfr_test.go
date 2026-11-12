package manifests

import "testing"

func boolPtr(b bool) *bool { return &b }

// Unset (nil) must mean ON: Go-built objects (and servers created before
// the field existed, until the apiserver defaults them) never carry the
// CRD default, so the renderer itself has to treat nil as true.
func TestPDNSConfigDisableAXFRDefaultsOn(t *testing.T) {
	for name, v := range map[string]*bool{"unset": nil, "true": boolPtr(true)} {
		t.Run(name, func(t *testing.T) {
			s := testServer()
			s.Spec.DisableAXFR = v
			conf := PDNSConfig(s)
			if lineIndex(conf, "disable-axfr=yes") < 0 {
				t.Fatalf("expected the exact line disable-axfr=yes, got:\n%s", conf)
			}
			if countLinesWithPrefix(conf, "disable-axfr") != 1 {
				t.Fatalf("expected exactly one disable-axfr line, got:\n%s", conf)
			}
		})
	}
}

// Explicit false renders NOTHING, so PowerDNS' own AXFR ACL applies
// (allow-axfr-ips, which a hidden primary sets via extraSettings).
func TestPDNSConfigDisableAXFRFalseRendersNothing(t *testing.T) {
	s := testServer()
	s.Spec.DisableAXFR = boolPtr(false)
	conf := PDNSConfig(s)
	if countLinesWithPrefix(conf, "disable-axfr") != 0 {
		t.Fatalf("disableAXFR=false must not render disable-axfr, got:\n%s", conf)
	}
	if ConfigHash(conf) == ConfigHash(PDNSConfig(testServer())) {
		t.Fatal("toggling disableAXFR must change the config hash so the pods roll")
	}
}

// It belongs to the managed block, ahead of any user extraSettings.
func TestPDNSConfigDisableAXFRBeforeExtraSettings(t *testing.T) {
	conf := PDNSConfig(serverWithExtras(map[string]string{"loglevel": "5"}))
	if lineIndex(conf, "disable-axfr=yes") > lineIndex(conf, "# spec.extraSettings") {
		t.Fatalf("disable-axfr must render before the extraSettings block, got:\n%s", conf)
	}
}

// AXFR-only extraSettings under the default disableAXFR are a silent
// contradiction (the upgrade path of every existing hidden primary).
func TestAXFRSettingsIgnored(t *testing.T) {
	for name, tc := range map[string]struct {
		disable *bool
		extra   map[string]string
		want    int
	}{
		"hidden primary, default":       {nil, map[string]string{"primary": "yes", "allow-axfr-ips": "192.0.2.10/32"}, 2},
		"hidden primary, explicit true": {boolPtr(true), map[string]string{"primary": "yes"}, 1},
		"legacy master":                 {nil, map[string]string{"master": "yes"}, 1},
		"primary off":                   {nil, map[string]string{"primary": "no"}, 0},
		"hidden primary, transfers on":  {boolPtr(false), map[string]string{"primary": "yes", "allow-axfr-ips": "192.0.2.10/32"}, 0},
		"unrelated settings":            {nil, map[string]string{"loglevel": "5"}, 0},
	} {
		t.Run(name, func(t *testing.T) {
			s := serverWithExtras(tc.extra)
			s.Spec.DisableAXFR = tc.disable
			if got := AXFRSettingsIgnored(s); len(got) != tc.want {
				t.Fatalf("AXFRSettingsIgnored = %v, want %d keys", got, tc.want)
			}
		})
	}
}

// The field owns the setting (disable-axfr is in the reserved-key table
// test): a rejected extraSettings override must never reach pdns.conf.
func TestDisableAXFRExtraSettingsOverrideNotRendered(t *testing.T) {
	conf := PDNSConfig(serverWithExtras(map[string]string{"disable-axfr": "no"}))
	if lineIndex(conf, "disable-axfr=no") >= 0 || countLinesWithPrefix(conf, "disable-axfr") != 1 {
		t.Fatalf("rejected extraSettings entry must not be rendered, got:\n%s", conf)
	}
}
