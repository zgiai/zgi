class ZGIPCMRecorderProcessor extends AudioWorkletProcessor {
  process(inputs) {
    const channels = inputs[0];
    if (!channels || channels.length === 0) return true;

    const frameLength = channels[0].length;
    const mono = new Float32Array(frameLength);
    for (let channelIndex = 0; channelIndex < channels.length; channelIndex += 1) {
      const channel = channels[channelIndex];
      for (let frameIndex = 0; frameIndex < frameLength; frameIndex += 1) {
        mono[frameIndex] += channel[frameIndex] / channels.length;
      }
    }
    this.port.postMessage(mono, [mono.buffer]);
    return true;
  }
}

registerProcessor('zgi-pcm-recorder', ZGIPCMRecorderProcessor);
