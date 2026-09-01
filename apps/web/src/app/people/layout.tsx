import type { ReactNode } from "react";
import { WorkspaceShell } from "@/components/workspace-shell";

export default function PeopleLayout({ children }: { children: ReactNode }) {
  return <WorkspaceShell>{children}</WorkspaceShell>;
}
