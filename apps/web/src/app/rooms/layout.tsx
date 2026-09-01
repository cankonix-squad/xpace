import type { ReactNode } from "react";
import { WorkspaceShell } from "@/components/workspace-shell";

export default function RoomsLayout({ children }: { children: ReactNode }) {
  return <WorkspaceShell>{children}</WorkspaceShell>;
}
