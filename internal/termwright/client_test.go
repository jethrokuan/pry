package termwright_test

import (
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/termwright"
)

// These tests require `termwright` to be installed:
//   cargo install termwright
//
// They are skipped automatically if the binary is not found.

func skipIfNoTermwright() {
	if _, err := exec.LookPath("termwright"); err != nil {
		Skip("termwright binary not found — install with: cargo install termwright")
	}
}

var _ = Describe("Termwright Client", func() {
	Describe("Spawn and basic interaction", func() {
		It("can spawn a process, read screen, and send input", func() {
			skipIfNoTermwright()

			// Spawn a simple shell command that echoes and waits for input
			client, err := termwright.Spawn(80, 24, "bash", "-c",
				`echo "HELLO TERMWRIGHT"; read -p "Enter: " line; echo "GOT: $line"`)
			Expect(err).NotTo(HaveOccurred())
			defer client.Close()

			// Wait for initial output
			err = client.WaitForText("HELLO TERMWRIGHT", 5*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Read screen content
			screen, err := client.Screen()
			Expect(err).NotTo(HaveOccurred())
			Expect(screen).To(ContainSubstring("HELLO TERMWRIGHT"))

			// Type input and press Enter
			err = client.TypeStr("test input")
			Expect(err).NotTo(HaveOccurred())
			err = client.Press("Enter")
			Expect(err).NotTo(HaveOccurred())

			// Verify the response appeared
			err = client.WaitForText("GOT: test input", 5*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Wait for process to exit
			code, err := client.WaitForExit(5 * time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(Equal(0))
		})

		It("can check process status", func() {
			skipIfNoTermwright()

			client, err := termwright.Spawn(80, 24, "bash", "-c", `echo "done"; sleep 1`)
			Expect(err).NotTo(HaveOccurred())
			defer client.Close()

			err = client.WaitForText("done", 5*time.Second)
			Expect(err).NotTo(HaveOccurred())

			exited, _, err := client.Status()
			Expect(err).NotTo(HaveOccurred())
			// Process might still be running (sleep 1)
			_ = exited
		})

		It("can resize the terminal", func() {
			skipIfNoTermwright()

			client, err := termwright.Spawn(80, 24, "bash", "-c",
				`tput cols; tput lines; read`)
			Expect(err).NotTo(HaveOccurred())
			defer client.Close()

			err = client.WaitForText("80", 5*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Resize and verify
			err = client.Resize(120, 40)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
