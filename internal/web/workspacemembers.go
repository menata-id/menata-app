package web

// legacyAppRoleApplicationID names the Application whose role still mirrors into
// workspace_members.app_role, the pre-Fase-3b single-Application column migration 008
// deliberately keeps so a rollback loses nothing. Document Approval, because that column's stored
// values (approver/submitter/reviewer) are exactly its declared vocabulary and it was the only
// Application in existence when they were written.
//
// This constant disappears with the column, in the migration that drops it.
const legacyAppRoleApplicationID = "app_document_approval"
