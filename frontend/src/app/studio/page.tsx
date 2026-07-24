import { Suspense } from "react";
import { StudioWorkspace } from "../../components/studio/StudioWorkspace";

export default function StudioRoute() {
  return (
    <Suspense fallback={null}>
      <StudioWorkspace />
    </Suspense>
  );
}
