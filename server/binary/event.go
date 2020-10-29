package binary

import (
	"wsnet2/pb"

	"golang.org/x/xerrors"
)

//go:generate stringer -type=EvType
type EvType byte

const regularEvType = 30
const responseEvType = 128
const (
	// NewEvPeerReady : Peer準備完了イベント
	// payload:
	// | 24bit-be msg sequence number |
	EvTypePeerReady EvType = 1 + iota
	EvTypePong
)
const (
	// EvTypeJoined : クライアントが入室した
	// payload:
	//  - str8: client ID
	//  - Dict: properties
	EvTypeJoined EvType = regularEvType + iota

	// EvTypeLeft : クライアントが退室した
	// payload:
	//  - str8: client ID
	//  - str8: master client ID
	EvTypeLeft

	// EvTypeRoomProp : 部屋情報の変更
	// payload:
	// - Byte: flags (1=visible, 2=joinable, 4=watchable)
	// - UInt: search group
	// - UShort: max players
	// - UShort: client deadline (second)
	// - Dict: public props (modified keys only)
	// - Dict: private props (modified keys only)
	EvTypeRoomProp

	// EvTypeClientProp : クライアント情報の変更
	// payload:
	//  - str8: client ID
	//  - Dict: properties (modified keys only)
	EvTypeClientProp

	// EvTypeMasterSwitched : Masterクライアントが切替わった
	// payload:
	//  - str8: new master client ID
	EvTypeMasterSwitched

	// EvTypeMessage : その他の通常メッセージ
	// payload: (any)
	EvTypeMessage
)
const (
	// EvTypeSucceeded:
	// payload:
	//  - 24bit be: Msg sequence num
	EvTypeSucceeded EvType = responseEvType + iota

	// EvTypePermissionDenied : 権限エラー
	// payload:
	//  - 24bit be: Msg sequence num
	//  - marshaled bytes: original msg payload
	EvTypePermissionDenied

	// EvTypeTargetNotFound : あて先不明
	// payload:
	//  - 24bit be: Msg sequence num
	//  - List: client IDs
	//  - marshaled bytes: original msg payload
	EvTypeTargetNotFound
)

type Event interface {
	Type() EvType
	Payload() []byte
}

// Event from wsnet to client via websocket
//
// regular event binary format:
// | 8bit EvType | 32bit-be sequence number | payload ... |
//
type RegularEvent struct {
	etype   EvType
	payload []byte
}

func (ev *RegularEvent) Type() EvType    { return ev.etype }
func (ev *RegularEvent) Payload() []byte { return ev.payload }

func (ev *RegularEvent) Marshal(seqNum int) []byte {
	buf := make([]byte, len(ev.payload)+5)
	buf[0] = byte(ev.etype)
	put32(buf[1:], seqNum)
	copy(buf[5:], ev.payload)
	return buf
}

// ParseMsg parse binary data to Event struct
func UnmarshalEvent(data []byte) (Event, int, error) {
	if len(data) < 1 {
		return nil, 0, xerrors.Errorf("data length not enough: %v", len(data))
	}

	et := EvType(data[0])
	data = data[1:]

	if et < regularEvType {
		return &SystemEvent{et, data}, 0, nil
	}

	if len(data) < 4 {
		return nil, 0, xerrors.Errorf("data length not enough: %v", len(data))
	}
	seq := get32(data)
	data = data[4:]

	return &RegularEvent{et, data}, seq, nil
}

// SystemEvent (without sequence number)
// - EvTypePeerReady
// - EvTypePong
// binary format:
// | 8bit MsgType | payload ... |
//
type SystemEvent struct {
	etype   EvType
	payload []byte
}

func (ev *SystemEvent) Type() EvType    { return ev.etype }
func (ev *SystemEvent) Payload() []byte { return ev.payload }

func (ev *SystemEvent) Marshal() []byte {
	buf := make([]byte, len(ev.payload)+1)
	buf[0] = byte(ev.etype)
	copy(buf[1:], ev.payload)
	return buf
}

// NewEvPeerReady : Peer準備完了イベント
// wsnetが受信済みのMsgシーケンス番号を通知.
// これを受信後、クライアントはMsgを該当シーケンス番号から送信する.
// payload:
// | 24bit-be msg sequence number |
func NewEvPeerReady(seqNum int) *SystemEvent {
	payload := make([]byte, 3)
	put24(payload, seqNum)
	return &SystemEvent{
		etype:   EvTypePeerReady,
		payload: payload,
	}
}

func UnmarshalEvPeerReadyPayload(payload []byte) (int, error) {
	if len(payload) < 3 {
		return 0, xerrors.Errorf("data length not enough: %v", len(payload))
	}

	return get24(payload), nil
}

// NewEvPong : Pongイベント
// payload:
// - unsigned 64bit-be: timestamp on ping sent.
// - unsigned 32bit-be: watcher count in the room.
// - dict: last msg timestamps of each player.
func NewEvPong(pingtime uint64, watchers uint32, lastMsg Dict) *SystemEvent {
	payload := MarshalULong(pingtime)
	payload = append(payload, MarshalUInt(int(watchers))...)
	payload = append(payload, MarshalDict(lastMsg)...)

	return &SystemEvent{
		etype:   EvTypePong,
		payload: payload,
	}
}

type EvPongPayload struct {
	Timestamp uint64
	Watchers  uint32
	LastMsg   Dict
}

func UnmarshalEvPongPayload(payload []byte) (*EvPongPayload, error) {
	pp := EvPongPayload{}

	// timestamp
	d, l, e := UnmarshalAs(payload, TypeULong)
	if e != nil {
		return nil, xerrors.Errorf("Invalid EvPong payload (timestamp): %w", e)
	}
	pp.Timestamp = d.(uint64)
	payload = payload[l:]

	// watchers
	d, l, e = UnmarshalAs(payload, TypeUInt)
	if e != nil {
		return nil, xerrors.Errorf("Invalid EvPong payload (watchers): %w", e)
	}
	pp.Watchers = d.(uint32)
	payload = payload[l:]

	// lastmsg
	d, l, e = UnmarshalAs(payload, TypeDict, TypeNull)
	if e != nil {
		return nil, xerrors.Errorf("Invalid EvPong payload (lastmsg): %w", e)
	}
	if d != nil {
		pp.LastMsg = d.(Dict)
	}

	return &pp, nil
}

// NewEvJoind : 入室イベント
func NewEvJoined(cli *pb.ClientInfo) *RegularEvent {
	payload := MarshalStr8(cli.Id)
	payload = append(payload, cli.Props...) // cli.Props marshaled as TypeDict

	return &RegularEvent{EvTypeJoined, payload}
}

type EvJoinedPayload struct {
	ClientId    string
	ClientProps Dict
}

func UnmarshalEvJoinedPayload(payload []byte) (*EvJoinedPayload, error) {
	um := EvJoinedPayload{}

	// client id
	d, l, e := UnmarshalAs(payload, TypeStr8)
	if e != nil {
		return nil, xerrors.Errorf("Invalid EvJoined payload (client id): %w", e)
	}
	um.ClientId = d.(string)
	payload = payload[l:]

	// client props
	d, l, e = UnmarshalAs(payload, TypeDict, TypeNull)
	if e != nil {
		return nil, xerrors.Errorf("Invalid EvJoined payload (client props): %w", e)
	}
	if d != nil {
		um.ClientProps = d.(Dict)
	}

	return &um, nil
}

func NewEvLeft(cliId, masterId string) *RegularEvent {
	payload := MarshalStr8(cliId)
	payload = append(payload, MarshalStr8(masterId)...)

	return &RegularEvent{EvTypeLeft, payload}
}

type EvLeftPayload struct {
	ClientId string
	MasterId string
}

func UnmarshalEvLeftPayload(payload []byte) (*EvLeftPayload, error) {
	um := EvLeftPayload{}

	// client id
	d, l, e := UnmarshalAs(payload, TypeStr8)
	if e != nil {
		return nil, xerrors.Errorf("Invalid EvLeft payload (client id): %w", e)
	}
	um.ClientId = d.(string)
	payload = payload[l:]

	// master id
	d, l, e = UnmarshalAs(payload, TypeStr8)
	if e != nil {
		return nil, xerrors.Errorf("Invalid EvLeft payload (master id): %w", e)
	}
	um.MasterId = d.(string)

	return &um, nil
}

func NewEvRoomProp(cliId string, rpp *MsgRoomPropPayload) *RegularEvent {
	return &RegularEvent{EvTypeRoomProp, rpp.EventPayload}
}

func NewEvClientProp(cliId string, props []byte) *RegularEvent {
	payload := make([]byte, 0, len(cliId)+1+len(props))
	payload = append(payload, MarshalStr8(cliId)...)
	payload = append(payload, props...)

	return &RegularEvent{EvTypeClientProp, payload}
}

type EvClientPropPayload struct {
	ClientId    string
	ClientProps Dict
}

func UnmarshalEvClientPropPayload(payload []byte) (*EvClientPropPayload, error) {
	um := EvClientPropPayload{}

	// client id
	d, l, e := UnmarshalAs(payload, TypeStr8)
	if e != nil {
		return nil, xerrors.Errorf("Invalid EvClientProp payload (client id): %w", e)
	}
	um.ClientId = d.(string)
	payload = payload[l:]

	// client props
	d, l, e = UnmarshalAs(payload, TypeDict, TypeNull)
	if e != nil {
		return nil, xerrors.Errorf("Invalid EvClientProp payload (client props): %w", e)
	}
	if d != nil {
		um.ClientProps = d.(Dict)
	}

	return &um, nil
}

func NewEvMasterSwitched(cliId, masterId string) *RegularEvent {
	return &RegularEvent{EvTypeMasterSwitched, MarshalStr8(masterId)}
}

func UnmarshalEvMasterSwitchedPayload(payload []byte) (string, error) {
	d, _, e := UnmarshalAs(payload, TypeStr8)
	if e != nil {
		return "", xerrors.Errorf("Invalid EvMasterSwitched payload (master id): %w", e)
	}

	return d.(string), nil
}

func NewEvMessage(cliId string, body []byte) *RegularEvent {
	payload := make([]byte, 0, len(cliId)+1+len(body))
	payload = append(payload, MarshalStr8(cliId)...)
	payload = append(payload, body...)
	return &RegularEvent{EvTypeMessage, payload}
}

// NewEvSucceeded : 成功イベント
func NewEvSucceeded(msg RegularMsg) *RegularEvent {
	payload := make([]byte, 3)
	put24(payload, msg.SequenceNum())
	return &RegularEvent{EvTypeSucceeded, payload}
}

// NewEvPermissionDenied : 権限エラー
// エラー発生の原因となったメッセージをそのまま返す
func NewEvPermissionDenied(msg RegularMsg) *RegularEvent {
	payload := make([]byte, 3+len(msg.Payload()))
	put24(payload, msg.SequenceNum())
	copy(payload[3:], msg.Payload())
	return &RegularEvent{EvTypePermissionDenied, payload}
}

// NewEvTargetNotFound : あて先不明
// 不明なClientのリストとエラー発生の原因となったメッセージをそのまま返す
func NewEvTargetNotFound(msg RegularMsg, cliIds []string) *RegularEvent {
	payload := make([]byte, 3, 3+len(msg.Payload()))
	put24(payload, msg.SequenceNum())
	payload = append(payload, MarshalStrings(cliIds)...)
	payload = append(payload, msg.Payload()...)
	return &RegularEvent{EvTypeTargetNotFound, payload}
}
