import type { ReactNode } from "react";
import { WorkspaceShell } from "@/components/workspace-shell";

export default function RecordingsLayout({ children }: { children: ReactNode }) {
  return <WorkspaceShell>{children}</WorkspaceShell>;
}
