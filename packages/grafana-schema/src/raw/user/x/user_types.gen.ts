// THIS FILE IS GENERATED. EDITING IS FUTILE.
//
// Generated by:
//     kinds/gen.go
// Using jennies:
//     TSTypesJenny
//     LatestMajorsOrXJenny
//
// Run 'make gen-cue' from repository root to regenerate.

export interface User {
  /**
   * AccessControl metadata associated with a given resource.
   */
  accessControl: Record<string, unknown>;
  /**
   * AuthLabels is a list of authentication providers used (OAuth, SAML, LDAP...)
   */
  authLabels: Array<string>;
  /**
   * AvatarUrl is the user's avatar URL.
   */
  avatarUrl: string;
  /**
   * CreatedAt indicates when the user was created.
   */
  createdAt: number;
  /**
   * Email is the user's email.
   */
  email: string;
  /**
   * IsDisabled indicates if the user is disabled.
   */
  isDisabled: boolean;
  /**
   * IsDisabled indicates if the user is external.
   */
  isExternal: boolean;
  /**
   * IsGrafanaAdmin indicates if the user belongs to Grafana.
   */
  isGrafanaAdmin: boolean;
  /**
   * Login is the name used for login.
   */
  login: string;
  /**
   * Name is the user's name.
   */
  name: string;
  /**
   * OrgId is the org where the user belongs to.
   */
  orgId?: string;
  /**
   * Theme is Grafana theme used by the user.
   */
  theme?: string;
  /**
   * UpdatedAt indicates when the user was updated.
   */
  updatedAt: number;
}

export const defaultUser: Partial<User> = {
  authLabels: [],
};
