package paths

import "sync"

type PathEnd struct {
	clientId string
	connId   string
	port     string
	chanId   string
	seq      uint64
	lock     *sync.Mutex
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

func (pe *PathEnd) Seq() uint64 {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	return pe.seq
}
func (pe *PathEnd) SetSeq(seq uint64) {
	pe.lock.Lock()
	defer pe.lock.Unlock()
	pe.seq = seq
}

func NewPath(props *Props) *Path {
	return &Path{
		source: &PathEnd{
			clientId: props.SourceClient,
			connId:   props.SourceConnection,
			port:     props.SourcePort,
			chanId:   props.SourceChannel,
			seq:      props.SourceSequence,
			lock:     &sync.Mutex{},
		},
		dest: &PathEnd{
			clientId: props.DestClient,
			connId:   props.DestConnection,
			port:     props.DestPort,
			chanId:   props.DestChannel,
			seq:      props.DestSequence,
			lock:     &sync.Mutex{},
		},
	}
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
	SourceSequence   uint64
	DestChannel      string
	DestClient       string
	DestPort         string
	DestConnection   string
	DestSequence     uint64
}

func (p *Path) Source() *PathEnd {
	return p.source
}
func (p *Path) Dest() *PathEnd {
	return p.dest
}
