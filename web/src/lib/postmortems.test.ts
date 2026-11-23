import { expect, test } from "vitest";
import { hindsight } from "./postmortems";

test("the portal flags the same hindsight and blame wording as the server", () => {
  expect(
    hindsight("The on-call engineer should have rolled back.")[0].phrase,
  ).toBe("should have");
  expect(hindsight("Deploy FAILED TO check the canary.")[0].phrase).toBe(
    "failed to",
  );
  expect(hindsight("Classic human error.").map((f) => f.phrase)).toContain(
    "human error",
  );
  expect(hindsight("")).toEqual([]);
  // Whole words only: a blameless review is not blame.
  expect(
    hindsight("The blameless review found three contributing causes."),
  ).toEqual([]);
});
