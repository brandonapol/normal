package baseline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/brandonapol/normal/pkg/audit"
	"github.com/brandonapol/normal/pkg/config"
)

const FileName = "baseline.sealed.json"

type Sealed struct {
	Document  string `json:"document"`
	KeyID     string `json:"keyId,omitempty"`
	Signature string `json:"signature,omitempty"`
}

type ProblemKind string

const (
	ProblemUnreadable  ProblemKind = "unreadable"
	ProblemUnsigned    ProblemKind = "unsigned-baseline"
	ProblemForeignKey  ProblemKind = "unexpected-signing-key"
	ProblemBadSigning  ProblemKind = "invalid-signature"
	ProblemInvalid     ProblemKind = "invalid-baseline"
	ProblemNotBaseline ProblemKind = "not-a-baseline"
)

type Problem struct {
	Kind    ProblemKind `json:"kind"`
	Message string      `json:"message"`
}

func (p Problem) String() string { return fmt.Sprintf("%s (%s)", p.Message, p.Kind) }

func canonical(c config.Config) ([]byte, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

func Seal(c config.Config, signer audit.Signer) (Sealed, error) {
	document, err := canonical(c)
	if err != nil {
		return Sealed{}, fmt.Errorf("encoding baseline: %w", err)
	}
	sealed := Sealed{Document: string(document)}
	if signer == nil {
		return sealed, nil
	}
	signature, err := signer.Sign(document)
	if err != nil {
		return Sealed{}, fmt.Errorf("signing baseline: %w", err)
	}
	sealed.KeyID = signer.KeyID()
	sealed.Signature = audit.EncodeSignature(signature)
	return sealed, nil
}

func (s Sealed) Config() (config.Config, error) {
	return config.ParseConfig([]byte(s.Document))
}

func (s Sealed) Verify(publicKey []byte, now time.Time) []Problem {
	problems := make([]Problem, 0)

	if len(publicKey) > 0 {
		switch {
		case s.Signature == "":
			return append(problems, Problem{
				Kind:    ProblemUnsigned,
				Message: "the baseline carries no signature, so it did not ship with this image",
			})
		case s.KeyID != audit.KeyIDFor(publicKey):
			return append(problems, Problem{
				Kind: ProblemForeignKey,
				Message: fmt.Sprintf("the baseline is signed with key %s, expected %s",
					s.KeyID, audit.KeyIDFor(publicKey)),
			})
		default:
			if err := audit.VerifySignature(publicKey, []byte(s.Document), s.Signature); err != nil {
				return append(problems, Problem{Kind: ProblemBadSigning, Message: err.Error()})
			}
		}
	}

	document, err := config.ParseDocument([]byte(s.Document))
	if err != nil {
		return append(problems, Problem{Kind: ProblemUnreadable, Message: err.Error()})
	}
	for _, issue := range config.Validate(document, now) {
		problems = append(problems, Problem{
			Kind:    ProblemInvalid,
			Message: fmt.Sprintf("%s: %s", issue.Path, issue.Message),
		})
	}

	parsed, err := s.Config()
	if err == nil && parsed.Metadata.Revision != 0 {
		problems = append(problems, Problem{
			Kind: ProblemNotBaseline,
			Message: fmt.Sprintf("a sealed baseline starts at revision 0, this one claims %d",
				parsed.Metadata.Revision),
		})
	}

	return problems
}

type FileSystem interface {
	Read(ctx context.Context, path string) (string, error)
	Write(ctx context.Context, path, contents string) error
	Exists(ctx context.Context, path string) (bool, error)
}

func Path(root string) string { return root + "/" + FileName }

func Write(ctx context.Context, fs FileSystem, root string, sealed Sealed) error {
	raw, err := json.MarshalIndent(sealed, "", "  ")
	if err != nil {
		return err
	}
	return fs.Write(ctx, Path(root), string(raw)+"\n")
}

func Read(ctx context.Context, fs FileSystem, root string) (Sealed, bool, error) {
	path := Path(root)
	exists, err := fs.Exists(ctx, path)
	if err != nil || !exists {
		return Sealed{}, false, err
	}
	raw, err := fs.Read(ctx, path)
	if err != nil {
		return Sealed{}, true, err
	}
	var sealed Sealed
	if err := json.Unmarshal([]byte(raw), &sealed); err != nil {
		return Sealed{}, true, fmt.Errorf("%s is unreadable: %w", path, err)
	}
	return sealed, true, nil
}
