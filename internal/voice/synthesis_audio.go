package voice

import "context"

func synthesizedAudio(ctx context.Context, synthesizer Synthesizer, req TurnRequest, userText, responseText string) ([][]byte, AudioStream) {
	if synthesizer == nil {
		return nil, nil
	}

	synthesisReq := SynthesisRequest{
		SessionID: req.SessionID,
		DeviceID:  req.DeviceID,
		UserText:  userText,
		Text:      responseText,
	}

	if streaming, ok := synthesizer.(StreamingSynthesizer); ok {
		stream, err := streaming.StreamSynthesize(ctx, synthesisReq)
		if err == nil && stream != nil {
			return nil, stream
		}
	}

	result, err := synthesizer.Synthesize(ctx, synthesisReq)
	if err != nil || len(result.AudioPCM) == 0 {
		return nil, nil
	}
	return chunkPCM16(result.AudioPCM, result.SampleRateHz, result.Channels, 20), nil
}
