package procpool

import (
	"log"
	"os"
	"os/exec"
	"sync"
)

// Pool manages os.Process structs, added to it.
type Pool struct {
	processes []*os.Process
	*sync.Mutex
}

// NewPool constructs new Pool object.
func NewPool() Pool {
	return Pool{[]*os.Process{}, &sync.Mutex{}}
}

// Command is a wrapper of exec.Command, which also registers Cmd's
// process in current Pool.
func (p *Pool) Command(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	p.AddProcess(cmd.Process)
	return cmd
}

// AddProcess adds given process to current Pool's list with synchronization.
func (p *Pool) AddProcess(process *os.Process) {
	p.Lock()
	defer p.Unlock()
	p.processes = append(p.processes, process)
}

// Clear kills all listed in current Pool processes and clears it's internal processes
// list.
func (p *Pool) Clear() (err error) {
	p.Lock()
	defer p.Unlock()
	defer func() {
		p.processes = []*os.Process{}
	}()
	for _, v := range p.processes {
		err = v.Kill()
		if err != nil {
			log.Printf("Can't kill pid %v, error message: %v\n", v.Pid, err.Error())
		}
	}
	return
}
