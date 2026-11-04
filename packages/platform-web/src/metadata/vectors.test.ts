import { describe, expect, test } from "bun:test";
import { testPattern } from "./index";
import {
  type Case,
  loadVectors,
  runConditionCases,
  runDecimalCases,
  runExprCases,
  runLabelCases,
  short,
} from "./vector-runner";

describe("korpus: conditions", () => runConditionCases(loadVectors("conditions.json")));
describe("korpus: expressions", () => runExprCases(loadVectors("expressions.json")));
describe("korpus: labels", () => runLabelCases(loadVectors("labels.json")));

describe("korpus: decimals", () => runDecimalCases(loadVectors("decimals.json")));

describe("korpus: patterns", () => {
  for (const c of loadVectors("patterns.json") as Case[]) {
    if (c.valid === false) continue; // validator subset ada di Go (metadata:check); klien hanya menjalankan pola sah
    test(short(c.name), () => {
      for (const input of c.inputs as { value: string; match: boolean | null; reason?: string }[]) {
        const got = testPattern(c.pattern, input.value);
        expect(got.match).toBe(input.match);
        if (input.reason !== undefined) expect(got.reason as string | undefined).toBe(input.reason);
      }
    });
  }
});
