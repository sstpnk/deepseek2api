# Tool-Call Semantics

DS2API accepts OpenAI-compatible `tools` on chat and responses requests and
maps tool-use instructions into the prompt sent to DeepSeek.

## Prompt Format

The preferred internal instruction format is DSML:

```text
<|DSML|tool_calls>
<|DSML|invoke name="tool_name">
<|DSML|parameter name="argument">value</|DSML|parameter>
</|DSML|invoke>
</|DSML|tool_calls>
```

The parser also accepts the older XML-shaped form without the `|DSML|` marker.

## Streaming

`internal/toolstream` buffers possible tool-call markup while streaming. Plain
text is emitted as normal content; complete tool calls are emitted as OpenAI
tool-call deltas.

## Non-Streaming

`internal/toolcall` extracts complete tool-call markup from collected output and
returns OpenAI-compatible tool call objects.
