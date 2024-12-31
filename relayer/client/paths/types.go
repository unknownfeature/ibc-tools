package paths

import "sync"

type PathEnd struct {
	clientId string
	connId   string
	port     string
	chanId   string
	upgrade  bool
	height   int64
	lock     *sync.Mutex
}

func NewPathEnd(client, port, connection, channel string, upgrade bool) *PathEnd {
	return &PathEnd{
		lock:     new(sync.Mutex),
		port:     port,
		connId:   connection,
		clientId: client,
		chanId:   channel,
		upgrade:  upgrade,
	}
}

func (pe *PathEnd) Height() int64 {
	pe.lock.Lock()
	defer pe.lock.Unlock()

	return pe.height
}

func (pe *PathEnd) SetHeight(height int64) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	pe.height = height
}
func (pe *PathEnd) ClientId() string {

	pe.lock.Lock()
	defer pe.lock.Unlock()

	return pe.clientId
}
func (pe *PathEnd) SetClientId(clientId string) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	pe.clientId = clientId
}
func (pe *PathEnd) ConnId() string {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	return pe.connId
}
func (pe *PathEnd) SetConnId(connId string) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	pe.connId = connId
}
func (pe *PathEnd) Port() string {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	return pe.port
}
func (pe *PathEnd) SetPort(port string) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	pe.port = port
}
func (pe *PathEnd) ChanId() string {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	return pe.chanId
}
func (pe *PathEnd) SetChanId(chanId string) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	pe.chanId = chanId
}

func (pe *PathEnd) Upgrade() bool {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	return pe.upgrade
}

func (pe *PathEnd) SetUpgrade(upgrade bool) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	pe.upgrade = upgrade
}

type Path struct {
	source *PathEnd
	dest   *PathEnd
}

type Props struct {
	SourceChannel    string
	SourceClient     string
	SourcePort       string
	SourceConnection string
	SourceUpgrade    bool
	DestChannel      string
	DestClient       string
	DestPort         string
	DestConnection   string
	DestUpgrade      bool
}

func NewPath(props *Props) *Path {
	return &Path{
		source: NewPathEnd(props.SourceClient, props.SourcePort, props.SourceConnection, props.SourceChannel, props.SourceUpgrade),
		dest:   NewPathEnd(props.DestClient, props.DestPort, props.DestConnection, props.DestChannel, props.DestUpgrade),
	}
}

func (p *Path) Source() *PathEnd {
	return p.source
}
func (p *Path) Dest() *PathEnd {
	return p.dest
}
