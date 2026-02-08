import { PageHeader } from "@/components/shared/page-header"
import { SettingsPanel } from "@/components/admin/settings-panel"

export default function AdminSettingsPage() {
  return (
    <>
      <PageHeader
        title="Settings"
        description="Instance-wide configuration"
      />
      <SettingsPanel />
    </>
  )
}
