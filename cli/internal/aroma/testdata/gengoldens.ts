/**
 * Generates the differential goldens in testdata/goldens/core.json by running
 * the TypeScript reference implementation of Aroma over the same toy corpus
 * the Go tests use. Run from this directory with the reference checkout
 * path as the first argument (the reference needs `npm install` once for its
 * lezer dependencies):
 *
 *   node --experimental-strip-types gengoldens.ts /tmp/aroma-ts
 *
 * Node runs TypeScript directly through type stripping; no dependencies are
 * installed. The output header pins the reference commit so a golden can be
 * traced to the exact code that produced it.
 *
 * Copyright 2026 EstebanForge <esteban at attitude dot cl>
 * SPDX-License-Identifier: Apache-2.0
 */
import { execSync } from "node:child_process";
import { writeFileSync, mkdirSync } from "node:fs";
import { join } from "node:path";

const reference = process.argv[2] ?? "/tmp/aroma-ts";
const { buildIndex, queryFromSnippet, search, recommend, queriesFromLines } = await import(
  join(reference, "src/index.ts")
);
const { toy } = await import(join(reference, "tests/toy.ts"));
const { featurize } = await import(join(reference, "src/features.ts"));

const sha = execSync(`git -C ${reference} rev-parse HEAD`).toString().trim();

const LISTING_4 = `
[ if [ ( [ $view instanceof ViewGroup ] ) ]
  [ { [ for ( [ [ int [ $i = 0 ] ] ; [ $i < [ [ ( [ ( ViewGroup ) $view ] ) ] . [ getChildCount ( ) ] ] ] ; [ $i ++ ] ] )
        [ { [ [ View [ $innerView = [ [ ( [ ( ViewGroup ) $view ] ) ] . [ getChildAt ( $i ) ] ] ] ] ; ] } ] ] } ] ]
`;

const method = (name, ...statements) => ({
  path: `${name}.toy`,
  source: ["method " + name, ...statements, "end"].join("\n"),
});
const call = (name) => `[ [ ${name} ( ) ] ; ]`;
const calls = (name, ...names) => method(name, ...names.map(call));
const family = (prefix, ...shared) =>
  [1, 2, 3].map((number) =>
    calls(`${prefix}${number}`, "open", "read", ...shared, `${prefix}${number}_only`),
  );

// The build order is the reference test's; the id tie-break makes it
// significant.
const [x1, y1, x2, y2, x3, y3] = [...family("x", "flush", "close"), ...family("y", "log", "retry")]
  .sort((a, b) => (a.path < b.path ? -1 : 1));
const families = buildIndex([x1, y1, x2, y2, x3, y3], { languages: [toy] });
const openRead = queryFromSnippet(families, [call("open"), call("read")].join("\n"), { language: "toy" });

const single = buildIndex([calls("same", "open", "read"), calls("one_more", "open", "read", "x")], { languages: [toy] });
const singleQuery = queryFromSnippet(single, [call("open"), call("read")].join("\n"), { language: "toy" });

const apart = buildIndex(
  [calls("m1", "a", "b", "c", "x", "y", "z"), calls("m2", "b", "c", "d", "x", "y", "z")],
  { languages: [toy] },
);
const apartQuery = queryFromSnippet(apart, [call("a"), call("b"), call("c"), call("d")].join("\n"), { language: "toy" });

const mixed = buildIndex(
  [calls("whole", "a", "b", "c", "x", "y", "z", "w"), calls("partial", "a", "b", "x", "y", "z", "w")],
  { languages: [toy] },
);
const mixedQuery = queryFromSnippet(mixed, [call("a"), call("b"), call("c")].join("\n"), { language: "toy" });

const swap = buildIndex(
  [method("m2", "[ if [ ( [ z < 0 ] ) ]", "[ if [ ( [ y > 1 ] ) ]", "[ [ w = 4 ] ; ] ] ]", "[ [ v = 10 ] ; ]")],
  { languages: [toy] },
);
const swapQuery = queryFromSnippet(
  swap,
  ["[ [ x = 1 ] ; ]", "[ if [ ( [ y > 1 ] ) ] [ if [ ( [ z < 0 ] ) ] [ [ w = 4 ] ; ] ] ]"].join("\n"),
  { language: "toy" },
);

const rerank = buildIndex([calls("pair", "a", "b"), calls("triple", "a", "a", "a")], { languages: [toy] });
const rerankQuery = queryFromSnippet(
  rerank,
  [call("a"), call("a"), call("a"), call("b")].join("\n"),
  { language: "toy" },
);

const goldens = {
  _generated_from: {
    reference: "https://github.com/L9Labs/aroma-typescript",
    commit: sha,
    generator: "scripts/gengoldens.ts",
  },
  listing4Tokens: featurize(toy.parseSnippet(LISTING_4)).map((entry) => ({
    text: entry.token.text,
    local: entry.token.local,
    features: [...entry.features].sort(),
  })),
  search: {
    swap: search(swap, swapQuery),
    rerank: search(rerank, rerankQuery),
  },
  recommend: {
    families: recommend(families, openRead),
    familiesMinSupport1: recommend(families, openRead, { minSupport: 1 }),
    single: recommend(single, singleQuery, { minSupport: 1, minExpansion: 1.4 }),
    apart: recommend(apart, apartQuery, { minSimilarity: 0.3 }),
    mixed: recommend(mixed, mixedQuery, { minSimilarity: 0.4, minCohesion: 0.4 }),
  },
  lines: queriesFromLines(families, x1, [2, 3]).map((query) => ({
    size: query.size,
    origin: query.origin,
    lines: query.lines,
    results: search(families, query),
  })),
};

mkdirSync("testdata/goldens", { recursive: true });
writeFileSync("testdata/goldens/core.json", JSON.stringify(goldens, null, 2) + "\n");
console.log("goldens written from reference commit " + sha);
