import { describe } from "bun:test";
import {
  loadVectors,
  runConditionCases,
  runDecimalCases,
  runExprCases,
  runLabelCases,
} from "./vector-runner";

// Paritas dua sisi (Requirement 4.4, 22.2): korpus acak berseed dibangkitkan dan dikomit oleh test Go
// (sdk/metadata/parity_test.go); evaluator TypeScript harus menghasilkan hasil identik untuk setiap kasus.
describe("paritas Go: conditions acak", () =>
  runConditionCases(loadVectors("random/conditions.json")));
describe("paritas Go: expressions acak", () =>
  runExprCases(loadVectors("random/expressions.json")));
describe("paritas Go: labels acak", () => runLabelCases(loadVectors("random/labels.json")));
describe("paritas Go: decimals acak", () => runDecimalCases(loadVectors("random/decimals.json")));
