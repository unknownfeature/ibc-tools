package processor

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	chantypes "github.com/cosmos/ibc-go/v9/modules/core/04-channel/types"
	"github.com/cosmos/relayer/v2/relayer/provider"
	"go.uber.org/zap/zapcore"
)

var _ zapcore.ObjectMarshaler = packetIBCMessage{}
var _ zapcore.ObjectMarshaler = channelIBCMessage{}
var _ zapcore.ObjectMarshaler = connectionIBCMessage{}
var _ zapcore.ObjectMarshaler = clientICQMessage{}

// pathEndMessages holds the different IBC messages that
// will attempt to be sent to the pathEnd.
type pathEndMessages struct {
	connectionMessages []connectionIBCMessage
	channelMessages    []channelIBCMessage
	packetMessages     []packetIBCMessage
	clientICQMessages  []clientICQMessage
}

// packetIBCMessage holds a packet message's eventType and sequence along with it,
// useful for sending packets around internal to the PathProcessor.
type packetIBCMessage struct {
	info      provider.PacketInfo
	eventType string
}

// tracker creates a message tracker for message status
func (msg packetIBCMessage) tracker(assembled provider.RelayerMessage) messageToTrack {
	return packetMessageToTrack{
		msg:       msg,
		assembled: assembled,
	}
}

func (packetIBCMessage) msgType() string {
	return "packet"
}

// MarshalLogObject satisfies the zapcore.ObjectMarshaler interface
// so that you can use zap.Object("messages", r) when logging.
// This is typically useful when logging details about a partially sent result.
func (msg packetIBCMessage) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("type", msg.eventType)
	enc.AddString("src_port", msg.info.SourcePort)
	enc.AddString("src_channel", msg.info.SourceChannel)
	enc.AddString("dst_port", msg.info.DestPort)
	enc.AddString("dst_channel", msg.info.DestChannel)
	enc.AddUint64("sequence", msg.info.Sequence)
	enc.AddString("timeout_height", fmt.Sprintf(
		"%d-%d",
		msg.info.TimeoutHeight.RevisionNumber,
		msg.info.TimeoutHeight.RevisionHeight,
	))
	enc.AddUint64("timeout_timestamp", msg.info.TimeoutTimestamp)
	enc.AddString("data", base64.StdEncoding.EncodeToString(msg.info.Data))
	enc.AddString("ack", base64.StdEncoding.EncodeToString(msg.info.Ack))
	return nil
}

// channelKey returns channel key for new message by this eventType
// based on prior eventType.
func (msg packetIBCMessage) channelKey() (ChannelKey, error) {
	return PacketInfoChannelKey(msg.eventType, msg.info)
}

// channelIBCMessage holds a channel handshake message's eventType along with its details,
// useful for sending messages around internal to the PathProcessor.
type channelIBCMessage struct {
	eventType string
	info      provider.ChannelInfo
}

// tracker creates a message tracker for message status
func (msg channelIBCMessage) tracker(assembled provider.RelayerMessage) messageToTrack {
	return channelMessageToTrack{
		msg:       msg,
		assembled: assembled,
	}
}

func (channelIBCMessage) msgType() string {
	return "channel handshake"
}

// MarshalLogObject satisfies the zapcore.ObjectMarshaler interface
// so that you can use zap.Object("messages", r) when logging.
// This is typically useful when logging details about a partially sent result.
func (msg channelIBCMessage) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("type", msg.eventType)
	enc.AddString("port_id", msg.info.PortID)
	enc.AddString("channel_id", msg.info.ChannelID)
	enc.AddString("counterparty_port_id", msg.info.CounterpartyPortID)
	enc.AddString("counterparty_channel_id", msg.info.CounterpartyChannelID)
	enc.AddString("connection_id", msg.info.ConnID)
	enc.AddString("counterparty_connection_id", msg.info.CounterpartyConnID)
	enc.AddString("order", msg.info.Order.String())
	enc.AddString("version", msg.info.Version)
	return nil
}

// connectionIBCMessage holds a connection handshake message's eventType along with its details,
// useful for sending messages around internal to the PathProcessor.
type connectionIBCMessage struct {
	eventType string
	info      provider.ConnectionInfo
}

// tracker creates a message tracker for message status
func (msg connectionIBCMessage) tracker(assembled provider.RelayerMessage) messageToTrack {
	return connectionMessageToTrack{
		msg:       msg,
		assembled: assembled,
	}
}

func (connectionIBCMessage) msgType() string {
	return "connection handshake"
}

// MarshalLogObject satisfies the zapcore.ObjectMarshaler interface
// so that you can use zap.Object("messages", r) when logging.
// This is typically useful when logging details about a partially sent result.
func (msg connectionIBCMessage) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("type", msg.eventType)
	enc.AddString("client_id", msg.info.ClientID)
	enc.AddString("cntrprty_client_id", msg.info.CounterpartyClientID)
	enc.AddString("conn_id", msg.info.ConnID)
	enc.AddString("cntrprty_conn_id", msg.info.CounterpartyConnID)
	enc.AddString("cntrprty_commitment_prefix", msg.info.CounterpartyCommitmentPrefix.String())
	return nil
}

const (
	ClientICQTypeRequest  ClientICQType = "query_request"
	ClientICQTypeResponse ClientICQType = "query_response"
)

// clientICQMessage holds a client ICQ message info,
// useful for sending messages around internal to the PathProcessor.
type clientICQMessage struct {
	info provider.ClientICQInfo
}

// tracker creates a message tracker for message status
func (msg clientICQMessage) tracker(assembled provider.RelayerMessage) messageToTrack {
	return clientICQMessageToTrack{
		msg:       msg,
		assembled: assembled,
	}
}

func (clientICQMessage) msgType() string {
	return "client ICQ"
}

// MarshalLogObject satisfies the zapcore.ObjectMarshaler interface
// so that you can use zap.Object("messages", r) when logging.
// This is typically useful when logging details about a partially sent result.
func (msg clientICQMessage) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("type", msg.info.Type)
	enc.AddString("query_id", string(msg.info.QueryID))
	enc.AddString("request", string(msg.info.Request))
	return nil
}

// processingMessage tracks the state of a IBC message currently being processed.
type processingMessage struct {
	retryCount          uint64
	lastProcessedHeight uint64
	assembled           bool

	processing bool
	mu         sync.Mutex
}

func (m *processingMessage) isProcessing() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.processing
}

func (m *processingMessage) setProcessing(assembled bool, retryCount uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processing = true
	m.retryCount = retryCount
	m.assembled = assembled
}

func (m *processingMessage) setFinishedProcessing(height uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastProcessedHeight = height
	m.processing = false
}

type packetProcessingCache map[ChannelKey]packetChannelMessageCache
type packetChannelMessageCache map[string]*packetMessageSendCache

type packetMessageSendCache struct {
	mu sync.Mutex
	m  map[uint64]*processingMessage
}

func newPacketMessageSendCache() *packetMessageSendCache {
	return &packetMessageSendCache{
		m: make(map[uint64]*processingMessage),
	}
}

func (c *packetMessageSendCache) get(sequence uint64) *processingMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[sequence]
}

func (c *packetMessageSendCache) set(sequence uint64, height uint64, assembled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[sequence] = &processingMessage{
		processing:          true,
		lastProcessedHeight: height,
		assembled:           assembled,
	}
}

func (c packetChannelMessageCache) deleteMessages(toDelete ...map[string][]uint64) {
	for _, toDeleteMap := range toDelete {
		for message, toDeleteMessages := range toDeleteMap {
			if _, ok := c[message]; !ok {
				continue
			}
			c[message].mu.Lock()
			for _, sequence := range toDeleteMessages {
				delete(c[message].m, sequence)
			}
			c[message].mu.Unlock()
		}
	}
}

type channelProcessingCache map[string]*channelKeySendCache
type channelKeySendCache struct {
	mu sync.Mutex
	m  map[ChannelKey]*processingMessage
}

func newChannelKeySendCache() *channelKeySendCache {
	return &channelKeySendCache{
		m: make(map[ChannelKey]*processingMessage),
	}
}

func (c *channelKeySendCache) get(key ChannelKey) *processingMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[key]
}

func (c *channelKeySendCache) set(key ChannelKey, height uint64, assembled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = &processingMessage{
		processing:          true,
		lastProcessedHeight: height,
		assembled:           assembled,
	}
}

func (c channelProcessingCache) deleteMessages(toDelete ...map[string][]ChannelKey) {
	for _, toDeleteMap := range toDelete {
		for message, toDeleteMessages := range toDeleteMap {
			if _, ok := c[message]; !ok {
				continue
			}
			c[message].mu.Lock()
			for _, channel := range toDeleteMessages {
				delete(c[message].m, channel)
			}
			c[message].mu.Unlock()
		}
	}
}

type connectionProcessingCache map[string]*connectionKeySendCache
type connectionKeySendCache struct {
	mu sync.Mutex
	m  map[ConnectionKey]*processingMessage
}

func newConnectionKeySendCache() *connectionKeySendCache {
	return &connectionKeySendCache{
		m: make(map[ConnectionKey]*processingMessage),
	}
}

func (c *connectionKeySendCache) get(key ConnectionKey) *processingMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[key]
}

func (c *connectionKeySendCache) set(key ConnectionKey, height uint64, assembled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = &processingMessage{
		processing:          true,
		lastProcessedHeight: height,
		assembled:           assembled,
	}
}

func (c connectionProcessingCache) deleteMessages(toDelete ...map[string][]ConnectionKey) {
	for _, toDeleteMap := range toDelete {
		for message, toDeleteMessages := range toDeleteMap {
			if _, ok := c[message]; !ok {
				continue
			}
			c[message].mu.Lock()
			for _, connection := range toDeleteMessages {
				delete(c[message].m, connection)
			}
			c[message].mu.Unlock()
		}
	}
}

type clientICQProcessingCache struct {
	mu sync.Mutex
	m  map[provider.ClientICQQueryID]*processingMessage
}

func newClientICQProcessingCache() *clientICQProcessingCache {
	return &clientICQProcessingCache{
		m: make(map[provider.ClientICQQueryID]*processingMessage),
	}
}

func (c *clientICQProcessingCache) get(queryID provider.ClientICQQueryID) *processingMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[queryID]
}

func (c *clientICQProcessingCache) set(queryID provider.ClientICQQueryID, height uint64, assembled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[queryID] = &processingMessage{
		processing:          true,
		lastProcessedHeight: height,
		assembled:           assembled,
	}
}

func (c *clientICQProcessingCache) deleteMessages(toDelete ...provider.ClientICQQueryID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, queryID := range toDelete {
		delete(c.m, queryID)
	}
}

type pathEndPacketFlowResponse struct {
	SrcMessages []packetIBCMessage
	DstMessages []packetIBCMessage

	DstChannelMessage []channelIBCMessage
}

type pathEndChannelHandshakeResponse struct {
	SrcMessages []channelIBCMessage
	DstMessages []channelIBCMessage
}

type pathEndConnectionHandshakeResponse struct {
	SrcMessages []connectionIBCMessage
	DstMessages []connectionIBCMessage
}

func packetInfoChannelKey(p provider.PacketInfo) ChannelKey {
	return ChannelKey{
		ChannelID:             p.SourceChannel,
		PortID:                p.SourcePort,
		CounterpartyChannelID: p.DestChannel,
		CounterpartyPortID:    p.DestPort,
	}
}

type messageToTrack interface {
	// assembledMsg returns the assembled message ready to send.
	assembledMsg() provider.RelayerMessage

	// msgType returns a human readable string for logging describing the message type.
	msgType() string

	// satisfies zapcore.ObjectMarshaler interface for use with zap.Object().
	MarshalLogObject(enc zapcore.ObjectEncoder) error
}

type packetMessageToTrack struct {
	msg       packetIBCMessage
	assembled provider.RelayerMessage
}

func (t packetMessageToTrack) assembledMsg() provider.RelayerMessage {
	return t.assembled
}

func (t packetMessageToTrack) msgType() string {
	return t.msg.msgType()
}

func (t packetMessageToTrack) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	return t.msg.MarshalLogObject(enc)
}

type connectionMessageToTrack struct {
	msg       connectionIBCMessage
	assembled provider.RelayerMessage
}

func (t connectionMessageToTrack) assembledMsg() provider.RelayerMessage {
	return t.assembled
}

func (t connectionMessageToTrack) msgType() string {
	return t.msg.msgType()
}

func (t connectionMessageToTrack) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	return t.msg.MarshalLogObject(enc)
}

type channelMessageToTrack struct {
	msg       channelIBCMessage
	assembled provider.RelayerMessage
}

func (t channelMessageToTrack) assembledMsg() provider.RelayerMessage {
	return t.assembled
}

func (t channelMessageToTrack) msgType() string {
	return t.msg.msgType()
}

func (t channelMessageToTrack) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	return t.msg.MarshalLogObject(enc)
}

type clientICQMessageToTrack struct {
	msg       clientICQMessage
	assembled provider.RelayerMessage
}

func (t clientICQMessageToTrack) assembledMsg() provider.RelayerMessage {
	return t.assembled
}

func (t clientICQMessageToTrack) msgType() string {
	return t.msg.msgType()
}

func (t clientICQMessageToTrack) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	return t.msg.MarshalLogObject(enc)
}

// orderFromString parses a string into a channel order byte.
func orderFromString(order string) chantypes.Order {
	switch strings.ToUpper(order) {
	case chantypes.UNORDERED.String():
		return chantypes.UNORDERED
	case chantypes.ORDERED.String():
		return chantypes.ORDERED
	default:
		return chantypes.NONE
	}
}
