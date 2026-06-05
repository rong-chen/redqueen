package utils

import (
	"math"
)

const (
	// VAD parameters
	energyThreshold = 50.0 // Adjust based on background noise
	frameSizeMs     = 20   // 20ms frames
	sampleRate      = 16000
	samplesPerFrame = (sampleRate * frameSizeMs) / 1000 // 320 samples for 16kHz
)

// calculateEnergy calculates the short-time energy of a PCM frame
func calculateEnergy(frame []int16) float64 {
	var sum float64
	for _, sample := range frame {
		val := float64(sample)
		sum += val * val
	}
	return math.Sqrt(sum / float64(len(frame)))
}

// TrimSilence removes silence from the beginning and end of a 16kHz PCM audio
func TrimSilence(pcmData []int16) []int16 {
	if len(pcmData) == 0 {
		return pcmData
	}

	startIdx := 0
	endIdx := len(pcmData)

	// Find the start of speech
	for i := 0; i+samplesPerFrame <= len(pcmData); i += samplesPerFrame {
		frame := pcmData[i : i+samplesPerFrame]
		if calculateEnergy(frame) > energyThreshold {
			// Backtrack slightly to include the onset (e.g., 100ms padding)
			startIdx = i - (samplesPerFrame * 5)
			if startIdx < 0 {
				startIdx = 0
			}
			break
		}
	}

	// Find the end of speech
	for i := len(pcmData); i-samplesPerFrame >= 0; i -= samplesPerFrame {
		frame := pcmData[i-samplesPerFrame : i]
		if calculateEnergy(frame) > energyThreshold {
			// Add slight padding to include the offset
			endIdx = i + (samplesPerFrame * 5)
			if endIdx > len(pcmData) {
				endIdx = len(pcmData)
			}
			break
		}
	}

	if startIdx >= endIdx {
		// No speech detected, return original (or empty, but returning original is safer)
		return pcmData
	}

	return pcmData[startIdx:endIdx]
}
