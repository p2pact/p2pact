# Common dictionary

`common-1.zdict` — the common zstd dictionary (`spec/core.md` §5): 64 KB, zstd dictionary id
1345470787. A device compresses with it until it has trained its own dictionary for the device
it talks to.

Trained on the training half of [OpenAssistant oasst2](https://huggingface.co/datasets/OpenAssistant/oasst2)
(Apache-2.0): what people write to assistants and what assistants answer, in many languages,
each text cut to 4 KB, 4 MB per side. The other half is what the benchmarks in
[p2pact-tests](https://github.com/p2pact/p2pact-tests) send, so the dictionary is never measured
on the texts it learned. To reproduce it byte for byte (zstd 1.5.7):

```
# in p2pact-tests
go run ./cmd/p2pact-bench samples /tmp/samples
zstd --train -r /tmp/samples --maxdict=65536 --dictID=1345470787 -o common-1.zdict
```
