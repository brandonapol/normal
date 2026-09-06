package baseline_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/brandonapol/normal/pkg/audit"
	"github.com/brandonapol/normal/pkg/baseline"
	"github.com/brandonapol/normal/pkg/config"
	"github.com/brandonapol/normal/pkg/engine"
)

var now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func signer(t *testing.T, offset byte) *audit.SoftwareSigner {
	t.Helper()
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i) + offset
	}
	made, err := audit.NewSoftwareSignerFromSeed(seed)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	return made
}

func kinds(problems []baseline.Problem) []string {
	out := make([]string, 0, len(problems))
	for _, problem := range problems {
		out = append(out, string(problem.Kind))
	}
	return out
}

func TestSealedBaselineVerifies(t *testing.T) {
	key := signer(t, 0)
	sealed, err := baseline.Seal(config.Baseline(), key)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if sealed.Signature == "" || sealed.KeyID != key.KeyID() {
		t.Fatal("sealing should sign the document")
	}
	if problems := sealed.Verify(key.PublicKey(), now); len(problems) > 0 {
		t.Fatalf("a freshly sealed baseline should verify, got %v", kinds(problems))
	}

	restored, err := sealed.Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if restored.Spec.Launcher.Columns != config.Baseline().Spec.Launcher.Columns {
		t.Fatal("the sealed document should round-trip")
	}
}

func TestEditedBaselineIsRejected(t *testing.T) {
	key := signer(t, 0)
	sealed, err := baseline.Seal(config.Baseline(), key)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	sealed.Document = strings.Replace(sealed.Document, `"columns":1`, `"columns":4`, 1)

	problems := sealed.Verify(key.PublicKey(), now)
	if len(problems) == 0 {
		t.Fatal("an edited baseline must not verify")
	}
	if problems[0].Kind != baseline.ProblemBadSigning {
		t.Fatalf("expected invalid-signature, got %v", kinds(problems))
	}
}

func TestUnsignedAndForeignBaselinesAreRejected(t *testing.T) {
	ours := signer(t, 0)
	theirs := signer(t, 100)

	unsigned, err := baseline.Seal(config.Baseline(), nil)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if problems := unsigned.Verify(ours.PublicKey(), now); len(problems) == 0 ||
		problems[0].Kind != baseline.ProblemUnsigned {
		t.Fatalf("expected unsigned-baseline, got %v", kinds(problems))
	}

	foreign, err := baseline.Seal(config.Baseline(), theirs)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if problems := foreign.Verify(ours.PublicKey(), now); len(problems) == 0 ||
		problems[0].Kind != baseline.ProblemForeignKey {
		t.Fatalf("expected unexpected-signing-key, got %v", kinds(problems))
	}
}

func TestBaselineMustSatisfyEveryInvariant(t *testing.T) {
	key := signer(t, 0)

	loosened := config.Baseline()
	loosened.Spec.Attention.InfiniteScroll.Webview.InjectShim = false

	sealed, err := baseline.Seal(loosened, key)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	problems := sealed.Verify(key.PublicKey(), now)
	if len(problems) == 0 {
		t.Fatal("a baseline that breaks an invariant must not verify, however well signed")
	}
	if problems[0].Kind != baseline.ProblemInvalid {
		t.Fatalf("expected invalid-baseline, got %v", kinds(problems))
	}
}

func TestBaselineMustStartAtRevisionZero(t *testing.T) {
	key := signer(t, 0)

	advanced := config.Baseline()
	advanced.Metadata.Revision = 7

	sealed, err := baseline.Seal(advanced, key)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	problems := sealed.Verify(key.PublicKey(), now)
	found := false
	for _, problem := range problems {
		if problem.Kind == baseline.ProblemNotBaseline {
			found = true
		}
	}
	if !found {
		t.Fatalf("a baseline at a non-zero revision is not a baseline, got %v", kinds(problems))
	}
}

func TestShippedBaselineSatisfiesTheAttentionInvariants(t *testing.T) {
	policy := config.Baseline().Spec.Attention.InfiniteScroll

	if policy.Enforcement == "" {
		t.Error("the shipped baseline must set scroll enforcement")
	}
	if !policy.Webview.InjectShim {
		t.Error("the shipped baseline must inject the webview shim")
	}
	if len(policy.Detectors) == 0 {
		t.Error("the shipped baseline must carry scroll detectors")
	}
	if len(policy.Exemptions) != 0 {
		t.Error("the shipped baseline must ship with no exemptions")
	}
	if policy.MaxAutoLoads != 0 {
		t.Errorf("the shipped baseline should auto-load nothing, got %d", policy.MaxAutoLoads)
	}
}

func TestRoundTripThroughStorage(t *testing.T) {
	key := signer(t, 0)
	ports := engine.NewMemoryPorts(engine.MemoryOptions{})
	ctx := context.Background()

	if _, found, err := baseline.Read(ctx, ports.FS, "/etc/normal"); err != nil || found {
		t.Fatalf("an absent baseline should report not-found, got found=%v err=%v", found, err)
	}

	sealed, err := baseline.Seal(config.Baseline(), key)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if writeErr := baseline.Write(ctx, ports.FS, "/etc/normal", sealed); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}

	loaded, found, err := baseline.Read(ctx, ports.FS, "/etc/normal")
	if err != nil || !found {
		t.Fatalf("Read: found=%v err=%v", found, err)
	}
	if problems := loaded.Verify(key.PublicKey(), now); len(problems) > 0 {
		t.Fatalf("a stored baseline should verify, got %v", kinds(problems))
	}
}
