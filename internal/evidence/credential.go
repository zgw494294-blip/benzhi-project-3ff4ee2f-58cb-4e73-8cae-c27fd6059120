package evidence

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"strata-proof/internal/domain"
)

type CredentialIssuer struct{ secret []byte }

func NewCredentialIssuer(secret string) *CredentialIssuer {
	return &CredentialIssuer{secret: []byte(secret)}
}

func (i *CredentialIssuer) Issue(id string, manifest domain.FrozenManifest, review domain.ReviewDecision, issuedBy string, now time.Time) domain.ResearchCredential {
	credential := domain.ResearchCredential{CredentialID: id, DossierID: manifest.DossierID, FrozenManifestDigest: manifest.Digest, ReviewDecisionID: review.ID, IssuedBy: strings.TrimSpace(issuedBy), IssuedAt: now.UTC(), Status: "active"}
	credential.VerificationCode = i.code(credential)
	return credential
}

func (i *CredentialIssuer) Reissue(id string, manifest domain.FrozenManifest, review domain.ReviewDecision, issuedBy, replacesID string, now time.Time) domain.ResearchCredential {
	credential := domain.ResearchCredential{CredentialID: id, DossierID: manifest.DossierID, FrozenManifestDigest: manifest.Digest, ReviewDecisionID: review.ID, IssuedBy: strings.TrimSpace(issuedBy), IssuedAt: now.UTC(), Status: "active", ReplacesCredentialID: replacesID}
	credential.VerificationCode = i.code(credential)
	return credential
}

func (i *CredentialIssuer) Verify(credential domain.ResearchCredential, manifest domain.FrozenManifest) (bool, string) {
	if credential.Status != "active" {
		return false, "凭据已撤销或失效"
	}
	if credential.DossierID != manifest.DossierID || credential.FrozenManifestDigest != manifest.Digest {
		return false, "凭据与冻结摘要不匹配"
	}
	if !hmac.Equal([]byte(credential.VerificationCode), []byte(i.code(credential))) {
		return false, "凭据验证码已被篡改"
	}
	return true, "凭据有效，冻结摘要一致"
}

func (i *CredentialIssuer) code(c domain.ResearchCredential) string {
	mac := hmac.New(sha256.New, i.secret)
	mac.Write([]byte(strings.Join([]string{c.CredentialID, c.DossierID, c.FrozenManifestDigest, c.ReviewDecisionID, c.IssuedBy, c.IssuedAt.UTC().Format(time.RFC3339Nano), c.Status, c.ReplacesCredentialID}, "|")))
	return strings.ToUpper(hex.EncodeToString(mac.Sum(nil))[:24])
}
