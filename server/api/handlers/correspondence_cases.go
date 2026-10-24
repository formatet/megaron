package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var errCorrespondenceCase = errors.New("case must belong to you and the destination's original counterpart; subject must be 1–120 characters")

// attachCorrespondenceTx runs in the departure transaction: a failed append
// cannot dispatch a bearer, reserve payment or create a case at the other end.
func attachCorrespondenceTx(ctx context.Context, tx pgx.Tx, letter, world, sender uuid.UUID, requested, subject string) (uuid.UUID, error) {
	var recipient uuid.UUID
	// Lock the destination through departure so its identity cannot race capture.
	if err := tx.QueryRow(ctx, `SELECT ds.owner_id FROM messengers m JOIN settlements ds ON ds.id=m.destination_id WHERE m.id=$1 AND m.world_id=$2 FOR SHARE OF ds`, letter, world).Scan(&recipient); err != nil {
		return uuid.Nil, errCorrespondenceCase
	}
	var caseID uuid.UUID
	if requested != "" {
		parsed, err := uuid.Parse(requested)
		if err != nil {
			return uuid.Nil, errCorrespondenceCase
		}
		if err = tx.QueryRow(ctx, `SELECT id FROM correspondence_cases WHERE id=$1 AND world_id=$2 AND ((first_player_id=$3 AND second_player_id=$4) OR (first_player_id=$4 AND second_player_id=$3))`, parsed, world, sender, recipient).Scan(&caseID); err != nil {
			return uuid.Nil, errCorrespondenceCase
		}
	} else {
		subject = strings.TrimSpace(subject)
		if subject == "" {
			subject = "Correspondence"
		}
		if len([]rune(subject)) > 120 {
			return uuid.Nil, errCorrespondenceCase
		}
		if err := tx.QueryRow(ctx, `INSERT INTO correspondence_cases(world_id,first_player_id,second_player_id,subject) VALUES($1,$2,$3,$4) RETURNING id`, world, sender, recipient, subject).Scan(&caseID); err != nil {
			return uuid.Nil, err
		}
	}
	_, err := tx.Exec(ctx, `INSERT INTO correspondence_letters(id,case_id,sender_id,recipient_id,sender_name,recipient_name,origin_id,destination_id,origin_name,destination_name,message_text,sent_at,status,trade_offer,delivered,returned)
 SELECT m.id,$2,m.sender_id,$3,COALESCE(sp.wanax_name,sp.username),COALESCE(rp.wanax_name,rp.username),COALESCE(m.origin_id,m.origin_unit_id),m.destination_id,COALESCE(os.name,'Nomadic Host'),ds.name,m.message_text,m.sent_at,m.status,m.trade_offer,m.status='delivered',m.status='arrived'
 FROM messengers m JOIN players sp ON sp.id=m.sender_id JOIN players rp ON rp.id=$3 JOIN settlements ds ON ds.id=m.destination_id LEFT JOIN settlements os ON os.id=m.origin_id WHERE m.id=$1`, letter, caseID, recipient)
	return caseID, err
}
