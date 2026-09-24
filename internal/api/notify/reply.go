package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Reply links (DECISIONS I-245). A question with fixed options carries one
// link per option in its ntfy buttons and its email. The token names the
// question, the option and the question's expiry, and is signed with the
// same platform key as the unsubscribe link under a separate domain, so no
// login is needed and nothing is stored per link. The token is not what
// makes a link single use: the question's state is, since the first answer
// closes it and every later click is refused.

// ErrReplyExpired is a well-signed token past its question's expiry.
var ErrReplyExpired = errors.New("reply: the question has expired")

// replyDomain separates reply MACs from unsubscribe MACs over one key.
const replyDomain = "repose-reply-v1\x00"

// ReplyToken signs the link that answers question id with option index opt.
func (u *Unsubscriber) ReplyToken(id uuid.UUID, opt int, expires time.Time) string {
	payload := make([]byte, 16+1+8)
	copy(payload, id[:])
	payload[16] = byte(opt)
	binary.BigEndian.PutUint64(payload[17:], uint64(expires.Unix()))
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(u.replyMAC(payload))
}

// VerifyReply recovers the question and option from a token.
func (u *Unsubscriber) VerifyReply(token string, now time.Time) (uuid.UUID, int, error) {
	p, s, ok := strings.Cut(token, ".")
	if !ok {
		return uuid.Nil, 0, errors.New("reply: malformed token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil || len(payload) != 25 {
		return uuid.Nil, 0, errors.New("reply: malformed token")
	}
	sig, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return uuid.Nil, 0, errors.New("reply: malformed token")
	}
	if !hmac.Equal(sig, u.replyMAC(payload)) {
		return uuid.Nil, 0, errors.New("reply: invalid token")
	}
	id, _ := uuid.FromBytes(payload[:16])
	if now.Unix() > int64(binary.BigEndian.Uint64(payload[17:])) {
		return id, int(payload[16]), ErrReplyExpired
	}
	return id, int(payload[16]), nil
}

func (u *Unsubscriber) replyMAC(payload []byte) []byte {
	h := hmac.New(sha256.New, u.key)
	h.Write([]byte(replyDomain))
	h.Write(payload)
	return h.Sum(nil)
}

// ReplyURL is the link for one option; via says which channel carried it
// (ntfy|email) and is not signed, since it only labels the answer.
func (u *Unsubscriber) ReplyURL(base string, id uuid.UUID, opt int, expires time.Time, via string) string {
	return fmt.Sprintf("%s/v1/questions/reply?token=%s&via=%s", strings.TrimRight(base, "/"), u.ReplyToken(id, opt, expires), url.QueryEscape(via))
}
