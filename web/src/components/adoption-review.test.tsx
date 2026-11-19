import { expect, test } from "vitest";
import { adoptionFindings } from "./adoption-review";
import type { Journey, Objective } from "@/lib/api";

const objective = (
  id: string,
  service: string,
  lifecycle?: string,
  retired = false,
) =>
  ({
    slo: {
      id,
      name: id,
      service,
      lifecycle,
      retirement: retired ? {} : undefined,
    },
  }) as unknown as Objective;
const journey = (name: string, links: [string, boolean][]) =>
  ({
    resource: { manifest: { spec: { displayName: name } } },
    links: links.map(([id, required]) => ({ link: { id, required } })),
  }) as unknown as Journey;

test("a crowded service is named with its count, ignoring experimental and archived objectives", () => {
  const objectives = [
    ...Array.from({ length: 6 }, (_, i) => objective(`a${i}`, "checkout")),
    objective("x", "checkout", "experimental"),
    objective("y", "checkout", "committed", true),
    ...Array.from({ length: 5 }, (_, i) => objective(`b${i}`, "search")),
  ];
  const f = adoptionFindings(objectives, [
    journey(
      "J",
      objectives.map((o) => [o.slo.id, true] as [string, boolean]),
    ),
  ]);
  const crowded = f.find((x) => x.id === "crowded_service")!;
  expect(crowded.items).toEqual(["checkout: 6 objectives"]);
});

test("no journeys is one finding, not one per objective", () => {
  const f = adoptionFindings([objective("a", "s"), objective("b", "s")], []);
  expect(f.map((x) => x.id)).toEqual(["no_journeys"]);
});

test("unlinked objectives and journeys without a required link are listed", () => {
  const f = adoptionFindings(
    [
      objective("linked", "s"),
      objective("loose", "s"),
      objective("exp", "s", "experimental"),
    ],
    [
      journey("Checkout", [["linked", false]]),
      journey("Search", [["linked", true]]),
    ],
  );
  expect(f.find((x) => x.id === "unlinked")!.items).toEqual(["loose"]);
  expect(f.find((x) => x.id === "no_required_link")!.items).toEqual([
    "Checkout",
  ]);
});

test("a healthy setup produces no advice", () => {
  const f = adoptionFindings(
    [objective("a", "s")],
    [journey("J", [["a", true]])],
  );
  expect(f).toEqual([]);
});
