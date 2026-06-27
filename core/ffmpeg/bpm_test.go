package ffmpeg

import (
	"encoding/binary"
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BPM analysis", func() {
	It("estimates BPM from regular audio pulses", func() {
		data := pulsePCM(120, 30, 11025)

		bpm := estimateBPMFromS16LE(data, 11025)

		Expect(bpm).To(BeNumerically("~", 120, 3))
	})

	It("returns zero when there is not enough audio", func() {
		Expect(estimateBPMFromS16LE(make([]byte, 11025), 11025)).To(Equal(0))
	})
})

func pulsePCM(bpm, seconds, sampleRate int) []byte {
	samples := seconds * sampleRate
	data := make([]byte, samples*2)
	period := int(math.Round(float64(sampleRate) * 60 / float64(bpm)))
	pulseLen := sampleRate / 50
	for i := 0; i < samples; i++ {
		pos := i % period
		var amp float64
		if pos < pulseLen {
			amp = math.Sin(math.Pi * float64(pos) / float64(pulseLen))
		}
		v := int16(amp * 28000)
		binary.LittleEndian.PutUint16(data[i*2:], uint16(v))
	}
	return data
}
