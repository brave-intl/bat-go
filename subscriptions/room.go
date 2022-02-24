package subscriptions

import (
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/square/go-jose/jwt"
)

type Room struct {
	Name         string      `db:"name"`
	HeadCount    string      `db:"head_count"`
	CreatedAt    pq.NullTime `db:"created_at"`
	TerminatedAt pq.NullTime `db:"terminated_at"`
	Tier         string      `db:"tier"`
}

type RoomClaims struct {
	*jwt.Claims
	Room    string                 `json:"room"`
	Aud     string                 `json:"aud"`
	Context map[string]interface{} `json:"context"`
}

type RoomRefreshClaims struct {
	*jwt.Claims
	Room      string                 `json:"room"`
	Aud       string                 `json:"aud"`
	Moderator bool                   `json:"moderator"`
	Group     bool                   `json:"group-room"`
	Context   map[string]interface{} `json:"context"`
}

func (r Room) makeClaim(mauP bool, tenantID string, moderator bool, isGroupRoom bool, userID string) RoomClaims {
	userInfo := map[string]string{
		"id":        uuid.New().String(),
		"moderator": strconv.FormatBool(moderator),
	}
	if len(userID) > 0 {
		userInfo["id"] = userID
		userInfo["email"] = fmt.Sprintf("%s@talk.brave.internal", userID)
	}

	customClaims := RoomClaims{
		Claims: &jwt.Claims{
			Issuer:    "chat",
			Subject:   tenantID,
			NotBefore: jwt.NewNumericDate(time.Now()),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour * time.Duration(3))),
		},
		Room: r.Name,
		Aud:  "jitsi",
		Context: map[string]interface{}{
			"features": map[string]string{
				"livestreaming": "true",
				"recording":     strconv.FormatBool(isGroupRoom),
			},
			"mauP": strconv.FormatBool(mauP),
			"user": userInfo,
			"x-brave-features": map[string]string{
				"group-room": strconv.FormatBool(isGroupRoom),
			},
		},
	}
	return customClaims
}

func (r Room) makeRefreshClaim(moderator bool, isGroupRoom bool) RoomRefreshClaims {
	customClaims := RoomRefreshClaims{
		Claims: &jwt.Claims{
			Issuer:    "chat",
			NotBefore: jwt.NewNumericDate(time.Now().Add(time.Hour * time.Duration(3))),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Expiry:    jwt.NewNumericDate(time.Now().Add(24 * time.Hour * time.Duration(45))),
		},
		Room:      r.Name,
		Aud:       "chat",
		Moderator: moderator,
		Group:     isGroupRoom,
		Context:   map[string]interface{}{},
	}
	return customClaims
}
