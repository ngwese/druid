package main

import (
	"bufio"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abiosoft/ishell"
	"github.com/tarm/serial"
)

type crow struct {
	Config *serial.Config
	Port   *serial.Port
}

func main() {
	deviceName := flag.String("device", "", "serial device")
	flag.Parse()

	crow := newCrow()

	shell := ishell.New()
	shell.SetHomeHistoryPath(".druid_history")
	addCommands(shell, crow)
	addGeneric(shell, crow)

	// open serial connection
	defer crow.Close()
	if err := crow.Open(deviceName); err != nil {
		log.Fatal(err)
	}

	// display welcome info
	shell.Printf("/// druid /// %s ///\n", crow.DeviceName())

	// run shell
	shell.Run()
}

func addCommands(shell *ishell.Shell, crow *crow) {
	shell.AddCmd(&ishell.Cmd{
		Name: "reset",
		Help: "reset crow",
		Func: func(c *ishell.Context) {
			crow.Reset()
			// FIXME: this borks the serial connection, need to re-open it
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "dump",
		Help: "prints the current user script saved in flash",
		Func: func(c *ishell.Context) {
			crow.PrintScript(c)
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "clear",
		Help: "clears the current script from flash",
		Func: func(c *ishell.Context) {
			if err := crow.ClearScript(); err != nil {
				c.Err(err)
			}
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "script",
		Help: "upload lua script",
		Func: func(c *ishell.Context) {
			var lines []string

			c.Println("args: ", c.Args)
			if len(c.Args) >= 1 {
				// load script from file
				f, err := os.Open(c.Args[0])
				defer f.Close()
				if err != nil {
					c.Err(err)
					return
				}
				scanner := bufio.NewScanner(f)
				for scanner.Scan() {
					lines = append(lines, scanner.Text())
				}
			} else {
				// load script interactively
				c.Println("enter script, end with '^D'.")
				input := c.ReadMultiLines(";") // force ^D to terminate script
				lines = strings.Split(input, "\n")
				lines = lines[:len(lines)-1] // remove trailing terminator
			}
			// debug
			c.Println("Done reading:")
			for n, line := range lines {
				c.Printf("%d: '%s'\n", n, line)
			}
			//c.Println(lines)

			if err := crow.StartScript(); err != nil {
				c.Err(err)
			}
			//defer crow.EndScript()
			//crow.Send("```")
			for _, line := range lines {
				crow.Send(line + "\n")
			}
			///crow.Send("```")
			//crow.Send("\n")
			crow.EndScript()
			printResponse(crow, c)
			//crow.Send("\n")
		},
	})
}

func addGeneric(shell *ishell.Shell, crow *crow) {
	shell.NotFound(func(c *ishell.Context) {
		// reconstitute command
		line := strings.Join(c.RawArgs, " ") + "\n"
		if err := crow.Send(line); err != nil {
			c.Err(err)
		}
		//c.Printf("sent '%s'", line)
		printResponse(crow, c)
	})
}

func newCrow() *crow {
	return &crow{}
}

func (c *crow) DefaultDevice() *string {
	matches, err := filepath.Glob("/dev/tty.usbmodem*")
	if err != nil || matches == nil {
		return nil
	}
	return &matches[0]
}

func (c *crow) Open(devicePath *string) error {
	if devicePath == nil || *devicePath == "" {
		devicePath = c.DefaultDevice()
	}
	// FIXME: this will panic if devicePath is nil
	c.Config = &serial.Config{
		Name:        *devicePath,
		Baud:        115200,
		ReadTimeout: time.Millisecond * 5,
	}
	port, err := serial.OpenPort(c.Config)
	c.Port = port
	return err
}

func (c *crow) Close() {
	if c.Port != nil {
		log.Println("closing", c.Port)
		c.Port.Close()
	}
}

func (c *crow) DeviceName() string {
	return c.Config.Name
}

func (c *crow) Reset() error {
	return c.Send("^^r\n")
}

func (c *crow) PrintScript(s *ishell.Context) error {
	if err := c.Send("^^p\n"); err != nil {
		return err
	}
	printResponse(c, s)
	return nil
}

func (c *crow) ClearScript() error {
	return c.Send("^^c\n")
}

func (c *crow) StartScript() error {
	return c.Send("^^s\n")
}

func (c *crow) EndScript() error {
	return c.Send("^^e\n")
}

func (c *crow) Send(input string) error {
	n, err := c.Write([]byte(input))
	log.Printf("send: %d, %p, '%q'", n, err, input)
	return err
}

func (c *crow) Write(data []byte) (int, error) {
	return c.Port.Write(data)
}

func (c *crow) Read(buf []byte) (int, error) {
	return c.Port.Read(buf)
}

func printResponse(crow *crow, c *ishell.Context) {
	// read response
	buf := make([]byte, 128)
	for {
		n, err := crow.Port.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			c.Err(err)
		}
		if n == 0 {
			break
		}
		c.Printf("%s", buf[:n])
	}
}
