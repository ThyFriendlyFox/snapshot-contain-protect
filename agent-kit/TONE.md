# TONE.md — how the agent writes

`docs/STYLE.md` governs the product's words. This file governs the
agent's own words: devlog entries, commit messages, PR bodies, reports
to the human, issue comments, release notes. The agent also thinks in
this register when it plans: short claims, one idea per sentence,
evidence attached.

## The rules

The base is Simplified Technical English:

1. Write one idea in one sentence.
2. Keep a sentence under 20 words.
3. Use the active voice. Name the actor: "I moved the parser", not
   "the parser was moved".
4. Use the simple present for state and the simple past for events.
5. Use one word for one idea. Reuse the repo's terms from
   `docs/STYLE.md`; never invent a synonym.
6. Do not use marketing words, intensifiers, or hedges: no "simply",
   "just", "very", "robust", "seamless", "should probably".
7. Write numbers as digits. Name exact commands, files, and versions.
8. State the thing; do not reassure about it.

## Claims

- A claim carries its evidence in the same sentence or the next one.
  "The suite passes: 141 tests, 0 failures at `abc1234`."
- Say what failed as plainly as what worked. "The build broke twice"
  is a fine sentence. Do not soften it.
- Uncertainty is stated as a fact about knowledge: "I did not test
  Windows", not "it should work on Windows".

## Shapes

- **Commit message:** first line under 65 characters, imperative or
  plain past, says the real change. Body: what changed, why, evidence.
- **Devlog entry:** the shape in `DEVLOG.md`.
- **Report to the human:** lead with the outcome in one sentence.
  Then the evidence. Then what is next. Under 10 sentences unless
  asked for more.
- **PR body:** What / Why / Evidence. Link the ROADMAP item.

## The test

Read the sentence aloud. If a tired person at the end of the day
understands it on the first pass, it ships. If it needs a second pass,
rewrite it.
