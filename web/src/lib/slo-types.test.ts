import { expect, test } from "vitest";
import { SLO_TYPES, classifySLO, countSLOTypes, sloType } from "./slo-types";

const m = (type: string) => ({
  measurement: { type },
});

test("the type comes from the saved definition, with good-minute taking precedence", () => {
  expect(classifySLO(m("availability")).primary).toBe("availability");
  expect(classifySLO(m("latency")).primary).toBe("latency");
  expect(classifySLO(m("sessions")).primary).toBe("sessions");
  expect(classifySLO({}).primary).toBe("custom");
  expect(classifySLO({ badInterval: { errorRate: 0.06 } }).primary).toBe(
    "good-minute",
  );
});

test("the book's SLI kinds classify from their measurement or judging rule", () => {
  for (const kind of ["correctness", "durability", "coverage", "freshness"])
    expect(classifySLO(m(kind)).primary).toBe(kind);
  expect(classifySLO({ badInterval: { minGoodEvents: 500 } }).primary).toBe(
    "throughput",
  );
  // An unknown type from a newer server never breaks the badge.
  expect(classifySLO(m("something-new")).primary).toBe("custom");
  expect(classifySLO(m("customers")).primary).toBe("custom");
});

test("per-customer reporting is an add-on only when customers are listed", () => {
  expect(
    classifySLO({ customers: { label: "a", customers: [] } }).addOns,
  ).toEqual([]);
  expect(
    classifySLO({
      ...m("availability"),
      customers: { label: "a", customers: [{ value: "x" }] },
    }),
  ).toEqual({ primary: "availability", addOns: ["customers"] });
});

test("counts group by primary type and every type has a documented home", () => {
  const counts = countSLOTypes([
    m("availability"),
    m("availability"),
    m("sessions"),
  ]);
  expect(counts.get("availability")).toBe(2);
  expect(counts.get("sessions")).toBe(1);
  for (const t of SLO_TYPES) {
    expect(t.docs).not.toMatch(/^\/|^http/);
    expect(sloType(t.id)).toBe(t);
    if (t.definedIn !== "Guided form" && t.id !== "custom")
      expect(t.snippet).toBeTruthy();
  }
});
