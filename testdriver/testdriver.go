// Package testdriver is a support package for plugins written using github.com/myitcv/govim
package testdriver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kr/pty"
	"github.com/myitcv/govim"
	"github.com/rogpeppe/go-internal/testscript"
)

type Driver struct {
	govimListener  net.Listener
	driverListener net.Listener
	govim          *govim.Govim

	cmd *exec.Cmd

	init func(*govim.Govim) error

	quitVim    chan bool
	quitGovim  chan bool
	quitDriver chan bool

	doneQuitVim    chan bool
	doneQuitGovim  chan bool
	doneQuitDriver chan bool

	doneInit chan bool

	errCh chan error
}

func NewDriver(env *testscript.Env, errCh chan error, init func(*govim.Govim) error) (*Driver, error) {
	res := &Driver{
		quitVim:    make(chan bool),
		quitGovim:  make(chan bool),
		quitDriver: make(chan bool),

		doneQuitVim:    make(chan bool),
		doneQuitGovim:  make(chan bool),
		doneQuitDriver: make(chan bool),

		doneInit: make(chan bool),

		init: init,

		errCh: errCh,
	}
	gl, err := net.Listen("tcp4", "localhost:0")
	if err != nil {
		res.errorf("failed to create listener for govim: %v", err)
	}
	dl, err := net.Listen("tcp4", ":0")
	if err != nil {
		res.errorf("failed to create listener for driver: %v", err)
	}

	res.govimListener = gl
	res.driverListener = dl

	env.Vars = append(env.Vars,
		"GOVIMTEST_SOCKET="+res.govimListener.Addr().String(),
		"GOVIMTESTDRIVER_SOCKET="+res.driverListener.Addr().String(),
	)

	vimrc, err := findLocalVimrc()
	if err != nil {
		return nil, fmt.Errorf("failed to find local vimrc: %v", err)
	}

	res.cmd = exec.Command("vim", "-u", vimrc)
	res.cmd.Env = env.Vars

	for i := len(env.Vars) - 1; i >= 0; i-- {
		if strings.HasPrefix(env.Vars[i], "WORK=") {
			res.cmd.Dir = strings.TrimPrefix(env.Vars[i], "WORK=")
			break
		}
	}

	return res, nil
}

func findLocalVimrc() (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("go", "list", "-f={{.Dir}}", "github.com/myitcv/govim/testdriver")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run %v: %v\n%s", strings.Join(cmd.Args, " "), err, stderr.Bytes())
	}
	dir := strings.TrimSpace(stdout.String())
	return filepath.Join(dir, "test.vim"), nil
}

func (d *Driver) Run() {
	go d.listenGovim()
	if _, err := pty.Start(d.cmd); err != nil {
		d.errorf("failed to start %v: %v", strings.Join(d.cmd.Args, " "), err)
	}
	go func() {
		if err := d.cmd.Wait(); err != nil {
			select {
			case <-d.quitVim:
				close(d.doneQuitVim)
			default:
				d.errorf("vim exited: %v", err)
			}
		}
	}()
	<-d.doneInit
}

func (d *Driver) Close() {
	close(d.quitVim)
	d.cmd.Process.Kill()
	<-d.doneQuitVim
	close(d.quitGovim)
	close(d.quitDriver)
	d.govimListener.Close()
	d.driverListener.Close()
	<-d.doneQuitGovim
	<-d.doneQuitDriver
}

func (d *Driver) errorf(format string, args ...interface{}) {
	err := fmt.Errorf(format, args...)
	panic(err)
	fmt.Printf("%v\n", err)
	d.errCh <- fmt.Errorf(format, args...)
}

func (d *Driver) listenGovim() {
	conn, err := d.govimListener.Accept()
	if err != nil {
		d.errorf("failed to accept connection on %v: %v", d.govimListener.Addr(), err)
	}
	g, err := govim.NewGoVim(conn, conn)
	if err != nil {
		d.errorf("failed to create govim: %v", err)
	}
	d.govim = g

	d.doInit()
	go d.listenDriver()

	if err := g.Run(); err != nil {
		select {
		case <-d.quitGovim:
		default:
			d.errorf("govim Run failed: %v", err)
		}
	}
	close(d.doneQuitGovim)
}

func (d *Driver) doInit() {
	if d.init != nil {
		go func() {
			if err := d.init(d.govim); err != nil {
				d.errorf("failed to run init: %v", err)
			}
		}()
	}
	close(d.doneInit)
}

func (d *Driver) listenDriver() {
	err := d.govim.DoProto(func() {
	Accept:
		for {
			conn, err := d.driverListener.Accept()
			if err != nil {
				select {
				case <-d.quitDriver:
					break Accept
				default:
					d.errorf("failed to accept connection to driver on %v: %v", d.driverListener.Addr(), err)
				}
			}
			dec := json.NewDecoder(conn)
			var args []interface{}
			if err := dec.Decode(&args); err != nil {
				d.errorf("failed to read command for driver: %v", err)
			}
			cmd := args[0]
			res := []interface{}{""}
			switch cmd {
			case "redraw":
				var force string
				if len(args) == 2 {
					force = args[1].(string)
				}
				if err := d.govim.ChannelRedraw(force == "force"); err != nil {
					d.errorf("failed to execute %v: %v", cmd, err)
				}
			case "ex":
				expr := args[1].(string)
				if err := d.govim.ChannelEx(expr); err != nil {
					d.errorf("failed to ChannelEx %v: %v", cmd, err)
				}
			case "normal":
				expr := args[1].(string)
				if err := d.govim.ChannelNormal(expr); err != nil {
					d.errorf("failed to ChannelNormal %v: %v", cmd, err)
				}
			case "expr":
				expr := args[1].(string)
				resp, err := d.govim.ChannelExpr(expr)
				if err != nil {
					d.errorf("failed to ChannelExpr %v: %v", cmd, err)
				}
				res = append(res, resp)
			case "call":
				fn := args[1].(string)
				resp, err := d.govim.ChannelCall(fn, args[2:]...)
				if err != nil {
					d.errorf("failed to ChannelCall %v: %v", cmd, err)
				}
				res = append(res, resp)
			default:
				d.errorf("don't yet know how to handle %v", cmd)
			}
			enc := json.NewEncoder(conn)
			if err := enc.Encode(res); err != nil {
				d.errorf("failed to encode response %v: %v", res, err)
			}
			conn.Close()
		}
	})

	if err != nil {
		d.errorf("%v", err)
	}
	close(d.doneQuitDriver)
}

// Vim is a sidecar that effectively drives Vim via a simple JSON-based
// API
func Vim() (exitCode int) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		exitCode = -1
		panic(r)
		fmt.Fprintln(os.Stderr, r)
	}()
	ef := func(format string, args ...interface{}) {
		panic(fmt.Sprintf(format, args...))
	}
	args := os.Args[1:]
	fn := args[0]
	var jsonArgs []string
	for i, a := range args {
		uq, err := strconv.Unquote("\"" + a + "\"")
		if err != nil {
			ef("failed to unquote %q: %v", a, err)
		}
		if i <= 1 {
			jsonArgs = append(jsonArgs, strconv.Quote(uq))
		} else {
			jsonArgs = append(jsonArgs, uq)
		}
	}
	jsonArgString := "[" + strings.Join(jsonArgs, ", ") + "]"
	var i []interface{}
	if err := json.Unmarshal([]byte(jsonArgString), &i); err != nil {
		ef("failed to json Unmarshal %q: %v", jsonArgString, err)
	}
	switch fn {
	case "redraw":
		// optional argument of force
		switch l := len(args[1:]); l {
		case 0:
		case 1:
			if args[1] != "force" {
				ef("unknown argument %q to redraw", args[1])
			}
		default:
			ef("redraw has a single optional argument: force; we saw %v", l)
		}
	case "ex", "normal", "expr":
		switch l := len(args[1:]); l {
		case 1:
			if _, ok := i[1].(string); !ok {
				ef("%v takes a string argument; saw %T", fn, i[1])
			}
		default:
			ef("%v takes a single argument: we saw %v", fn, l)
		}
	case "call":
		switch l := len(args[1:]); l {
		case 2:
			if _, ok := i[1].(string); !ok {
				ef("%v takes a string as its first argument; saw %T", fn, i[1])
			}
			if _, ok := i[2].([]interface{}); !ok {
				ef("%v takes a slice of values as its second argument; saw %T", fn, i[2])
			}
		default:
			ef("%v takes a two arguments: we saw %v", fn, l)
		}
	}
	addr := os.Getenv("GOVIMTESTDRIVER_SOCKET")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		ef("failed to connect to driver on %v: %v", addr, err)
	}
	if _, err := fmt.Fprintln(conn, jsonArgString); err != nil {
		ef("failed to send command %q to driver on: %v", jsonArgString, err)
	}
	dec := json.NewDecoder(conn)
	var resp []interface{}
	if err := dec.Decode(&resp); err != nil {
		ef("failed to decode response: %v", err)
	}
	if resp[0] != "" {
		ef("got error response: %v", resp[0])
	}
	if len(resp) == 2 {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(resp[1]); err != nil {
			ef("failed to format output of JSON: %v", err)
		}
	}
	conn.Close()
	return 0
}