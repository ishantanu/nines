import { createContext, useContext, type ReactNode } from "react";
import { featureOn } from "@/lib/maturity";

// The gated experimental features the current server turned on. Undefined
// until the workspace loads, which keeps gated features hidden meanwhile.
const FeaturesContext = createContext<readonly string[] | undefined>(undefined);

export function FeaturesProvider({
  enabled,
  children,
}: {
  enabled: readonly string[] | undefined;
  children: ReactNode;
}) {
  return (
    <FeaturesContext.Provider value={enabled}>
      {children}
    </FeaturesContext.Provider>
  );
}

export function useEnabledFeatures() {
  return useContext(FeaturesContext);
}

export function useFeature(id: string) {
  return featureOn(useContext(FeaturesContext), id);
}
