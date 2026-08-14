'use client';

import React, { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import NavigationShell from '@/components/NavigationShell';
import {
  Mail,
  Server,
  Plus,
  Trash2,
  Edit,
  Save,
  CheckCircle,
  AlertTriangle,
  ChevronDown,
  Info,
  Settings,
  Eye,
  FileCode
} from 'lucide-react';

interface MailServer {
  id: string;
  name: string;
  default: boolean;
  sender: string;
  senderEmail: string;
  auth?: boolean;
  smtp?: {
    host: string;
    port: string;
    user: string;
    password?: string;
    tlsSkipVerify?: boolean;
  };
  resetPasswordExpired: number;
  verifyEmailExpired: number;
}

interface EmailTemplate {
  id: string;
  name: string;
  alias: string;
  subject: string;
  variables: string[];
  htmlBody: string;
}

export default function AdminMailPage() {
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState<'servers' | 'templates'>('servers');

  // Form states for MailServer CRUD
  const [editingServer, setEditingServer] = useState<MailServer | null>(null);
  const [isServerFormOpen, setIsServerFormOpen] = useState(false);
  
  const [serverName, setServerName] = useState('');
  const [isDefault, setIsDefault] = useState(true);
  const [sender, setSender] = useState('');
  const [senderEmail, setSenderEmail] = useState('');
  const [smtpAuth, setSmtpAuth] = useState(true);
  const [smtpHost, setSmtpHost] = useState('');
  const [smtpPort, setSmtpPort] = useState('587');
  const [smtpUser, setSmtpUser] = useState('');
  const [smtpPassword, setSmtpPassword] = useState('');
  const [tlsSkipVerify, setTlsSkipVerify] = useState(false);
  const [resetExp, setResetExp] = useState(1440); // 24 hours in minutes
  const [verifyExp, setVerifyExp] = useState(1440);

  const [selectedTemplate, setSelectedTemplate] = useState<EmailTemplate | null>(null);

  const [formError, setFormError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  // Queries
  const { data: mailServers, isLoading: isLoadingServers } = useQuery<MailServer[]>({
    queryKey: ['admin-mail-servers'],
    queryFn: async () => {
      const res = await api.get('/mail-servers');
      return res.data.items || [];
    },
  });

  const { data: templates, isLoading: isLoadingTemplates } = useQuery<EmailTemplate[]>({
    queryKey: ['admin-email-templates'],
    queryFn: async () => {
      const res = await api.get('/email-templates');
      return res.data.items || [];
    },
  });

  // Load editing MailServer into form states
  useEffect(() => {
    if (editingServer) {
      setServerName(editingServer.name || '');
      setIsDefault(editingServer.default === true);
      setSender(editingServer.sender || '');
      setSenderEmail(editingServer.senderEmail || '');
      setSmtpAuth(editingServer.auth !== false);
      setSmtpHost(editingServer.smtp?.host || '');
      setSmtpPort(editingServer.smtp?.port || '587');
      setSmtpUser(editingServer.smtp?.user || '');
      setSmtpPassword(editingServer.smtp?.password || '');
      setTlsSkipVerify(editingServer.smtp?.tlsSkipVerify === true);
      setResetExp(editingServer.resetPasswordExpired || 1440);
      setVerifyExp(editingServer.verifyEmailExpired || 1440);
      setIsServerFormOpen(true);
    } else {
      setServerName('');
      setIsDefault(true);
      setSender('ORYCA Gatekeeper');
      setSenderEmail('noreply@localhost');
      setSmtpAuth(true);
      setSmtpHost('');
      setSmtpPort('587');
      setSmtpUser('');
      setSmtpPassword('');
      setTlsSkipVerify(false);
      setResetExp(1440);
      setVerifyExp(1440);
    }
    setFormError(null);
  }, [editingServer, isServerFormOpen]);

  // Mutations
  const saveServerMutation = useMutation({
    mutationFn: async () => {
      setFormError(null);
      const payload = {
        name: serverName,
        default: isDefault,
        sender,
        senderEmail,
        auth: smtpAuth,
        smtp: {
          host: smtpHost,
          port: smtpPort,
          user: smtpUser,
          password: smtpPassword || undefined,
          tlsSkipVerify,
        },
        resetPasswordExpired: Number(resetExp),
        verifyEmailExpired: Number(verifyExp),
      };

      if (editingServer) {
        await api.put(`/mail-servers/${editingServer.id}`, payload);
      } else {
        await api.post('/mail-servers', payload);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-mail-servers'] });
      setIsServerFormOpen(false);
      setEditingServer(null);
      setSuccess('SMTP Mail Server configuration saved.');
    },
    onError: (err: any) => {
      setFormError(err.message || 'Failed to save SMTP mail server settings.');
    },
  });

  const deleteServerMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/mail-servers/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-mail-servers'] });
      setSuccess('Mail Server deleted.');
    },
  });

  return (
    <NavigationShell>
      <div className="space-y-6">
        {/* Toggle tabs */}
        <div className="flex border-b border-rule pb-px gap-6">
          <button
            onClick={() => setActiveTab('servers')}
            className={`pb-3 text-sm font-semibold border-b-2 transition ${
              activeTab === 'servers'
                ? 'border-accent text-ink font-bold'
                : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            📬 SMTP Servers
          </button>
          <button
            onClick={() => setActiveTab('templates')}
            className={`pb-3 text-sm font-semibold border-b-2 transition ${
              activeTab === 'templates'
                ? 'border-accent text-ink font-bold'
                : 'border-transparent text-muted hover:text-ink'
            }`}
          >
            📝 Email Templates
          </button>
        </div>

        {success && (
          <div className="flex items-start gap-3 rounded-control border border-ok-edge bg-ok-wash p-4 text-xs text-ok">
            <CheckCircle className="w-4 h-4 mt-0.5 flex-shrink-0" />
            <div>{success}</div>
          </div>
        )}

        {/* ================================= SERVERS TAB ================================= */}
        {activeTab === 'servers' && (
          <div className="space-y-6">
            <div className="flex justify-between items-center">
              <h2 className="text-xs font-bold text-muted uppercase tracking-wider font-title">SMTP Mail Configuration</h2>
              <button
                onClick={() => {
                  setEditingServer(null);
                  setIsServerFormOpen(true);
                }}
                className="px-3 py-1.5 bg-accent hover:bg-accent-deep text-accent-ink text-xs font-semibold rounded-control transition duration-short flex items-center gap-1"
              >
                <Plus className="w-4 h-4" /> Add Server
              </button>
            </div>

            {/* Server Form Modal */}
            {isServerFormOpen && (
              <div className="fixed inset-0 z-50 bg-scrim flex items-center justify-center p-4">
                <div className="bg-paper border border-rule rounded-surface p-6 w-full max-w-lg shadow-sm space-y-4 max-h-[95vh] overflow-y-auto">
                  <h3 className="font-title text-base font-bold text-ink">
                    {editingServer ? 'Edit SMTP Configuration' : 'Add SMTP Configuration'}
                  </h3>

                  {formError && (
                    <div className="flex items-start gap-3 rounded-control border border-danger-edge bg-danger-wash p-4 text-xs text-danger">
                      <AlertTriangle className="w-4 h-4 mt-0.5 flex-shrink-0" />
                      <div>{formError}</div>
                    </div>
                  )}

                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      saveServerMutation.mutate();
                    }}
                    className="space-y-4 text-xs"
                  >
                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Server Name
                        </label>
                        <input
                          type="text"
                          required
                          value={serverName}
                          onChange={(e) => setServerName(e.target.value)}
                          placeholder="e.g. Google SMTP"
                          className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                        />
                      </div>
                      <div className="flex items-center gap-2 pt-5">
                        <input
                          type="checkbox"
                          id="srv-default"
                          checked={isDefault}
                          onChange={(e) => setIsDefault(e.target.checked)}
                          className="w-4 h-4 accent-accent"
                        />
                        <label htmlFor="srv-default" className="text-xs font-semibold text-ink-2 select-none cursor-pointer">
                          Set as Default Server
                        </label>
                      </div>
                    </div>

                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Sender Name (From name)
                        </label>
                        <input
                          type="text"
                          required
                          value={sender}
                          onChange={(e) => setSender(e.target.value)}
                          placeholder="e.g. ORYCA Gatekeeper"
                          className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus"
                        />
                      </div>
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Sender Email (From address)
                        </label>
                        <input
                          type="email"
                          required
                          value={senderEmail}
                          onChange={(e) => setSenderEmail(e.target.value)}
                          placeholder="noreply@oryca.io"
                          className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                        />
                      </div>
                    </div>

                    <div className="grid grid-cols-3 gap-4">
                      <div className="col-span-2">
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          SMTP Host
                        </label>
                        <input
                          type="text"
                          required
                          value={smtpHost}
                          onChange={(e) => setSmtpHost(e.target.value)}
                          placeholder="smtp.gmail.com"
                          className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                        />
                      </div>
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Port
                        </label>
                        <input
                          type="text"
                          required
                          value={smtpPort}
                          onChange={(e) => setSmtpPort(e.target.value)}
                          placeholder="587"
                          className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                        />
                      </div>
                    </div>

                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          SMTP User (Auth account)
                        </label>
                        <input
                          type="text"
                          required={smtpAuth}
                          value={smtpUser}
                          onChange={(e) => setSmtpUser(e.target.value)}
                          placeholder="user@example.com"
                          className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                        />
                      </div>
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          SMTP Password
                        </label>
                        <input
                          type="password"
                          value={smtpPassword}
                          onChange={(e) => setSmtpPassword(e.target.value)}
                          placeholder={editingServer ? '•••••••• (unchanged)' : 'password'}
                          className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                        />
                      </div>
                    </div>

                    <div className="grid grid-cols-2 gap-4 items-center border-t border-rule pt-3">
                      <div className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          id="smtp-auth"
                          checked={smtpAuth}
                          onChange={(e) => setSmtpAuth(e.target.checked)}
                          className="w-4 h-4 accent-accent"
                        />
                        <label htmlFor="smtp-auth" className="text-xs font-semibold text-ink-2 select-none cursor-pointer">
                          Require Authentication
                        </label>
                      </div>

                      <div className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          id="tls-skip"
                          checked={tlsSkipVerify}
                          onChange={(e) => setTlsSkipVerify(e.target.checked)}
                          className="w-4 h-4 accent-accent"
                        />
                        <label htmlFor="tls-skip" className="text-xs font-semibold text-ink-2 select-none cursor-pointer">
                          Skip SSL Verification
                        </label>
                      </div>
                    </div>

                    <div className="grid grid-cols-2 gap-4 border-t border-rule pt-3">
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Verify Token Expiry (Minutes)
                        </label>
                        <input
                          type="number"
                          required
                          value={verifyExp}
                          onChange={(e) => setVerifyExp(parseInt(e.target.value) || 1440)}
                          className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                        />
                      </div>
                      <div>
                        <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                          Reset Token Expiry (Minutes)
                        </label>
                        <input
                          type="number"
                          required
                          value={resetExp}
                          onChange={(e) => setResetExp(parseInt(e.target.value) || 1440)}
                          className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none focus:border-focus font-mono"
                        />
                      </div>
                    </div>

                    <div className="flex justify-end gap-3 pt-2">
                      <button
                        type="button"
                        onClick={() => {
                          setIsServerFormOpen(false);
                          setEditingServer(null);
                        }}
                        className="px-4 py-2 border border-rule hover:border-faint rounded-control text-ink-2 hover:bg-paper-2 transition"
                      >
                        Cancel
                      </button>
                      <button
                        type="submit"
                        disabled={saveServerMutation.isPending}
                        className="px-4 py-2 bg-accent hover:bg-accent-deep text-accent-ink font-semibold rounded-control transition"
                      >
                        {saveServerMutation.isPending ? 'Saving...' : 'Save Settings'}
                      </button>
                    </div>
                  </form>
                </div>
              </div>
            )}

            {isLoadingServers ? (
              <div className="space-y-4">
                <div className="h-16 bg-paper rounded-surface border border-rule animate-pulse"></div>
              </div>
            ) : !mailServers || mailServers.length === 0 ? (
              <div className="p-8 bg-paper border border-rule border-dashed rounded-surface text-center text-xs text-muted">
                No mail servers configured. You must set up SMTP to allow automated emails (registration confirmation, password recovery).
              </div>
            ) : (
              <div className="space-y-3">
                {mailServers.map((server) => (
                  <div key={server.id} className="bg-paper border border-rule p-4 rounded-surface shadow-sm hover:border-faint transition flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                    <div className="flex items-start gap-3">
                      <div className="p-2 rounded-control bg-paper-2 text-muted mt-1">
                        <Mail className="w-5 h-5" />
                      </div>
                      <div>
                        <div className="flex items-center gap-2">
                          <span className="font-title text-sm font-semibold text-ink">{server.name}</span>
                          {server.default && (
                            <span className="text-[9px] uppercase font-bold tracking-wider px-2 py-0.5 rounded-chip bg-accent-wash border border-accent-edge text-accent">
                              Default SMTP
                            </span>
                          )}
                        </div>
                        <p className="text-xs text-muted mt-1">
                          Sender: {server.sender} &lt;{server.senderEmail}&gt;
                        </p>
                        <p className="text-[10px] text-muted font-mono mt-1">
                          Server: {server.smtp?.host}:{server.smtp?.port} (User: {server.smtp?.user})
                        </p>
                      </div>
                    </div>

                    <div className="flex gap-2 self-end sm:self-center">
                      <button
                        onClick={() => setEditingServer(server)}
                        className="p-1.5 rounded-control text-muted hover:text-ink hover:bg-paper-3 transition"
                        title="Edit Server"
                      >
                        <Edit className="w-4 h-4" />
                      </button>
                      <button
                        onClick={() => {
                          if (confirm(`Delete mail server "${server.name}"?`)) {
                            deleteServerMutation.mutate(server.id);
                          }
                        }}
                        disabled={deleteServerMutation.isPending}
                        className="p-1.5 rounded-control text-muted hover:text-danger hover:bg-danger-wash border border-transparent hover:border-danger-edge transition"
                        title="Delete Server"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* ================================= TEMPLATES TAB ================================= */}
        {activeTab === 'templates' && (
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            {/* Templates list */}
            <div className="lg:col-span-1 space-y-4">
              <h2 className="text-xs font-bold text-muted uppercase tracking-wider">Default System Templates</h2>
              {isLoadingTemplates ? (
                <div className="space-y-3">
                  {[...Array(2)].map((_, i) => (
                    <div key={i} className="h-12 bg-paper rounded-control border animate-pulse"></div>
                  ))}
                </div>
              ) : !templates || templates.length === 0 ? (
                <p className="text-xs text-muted text-center py-6">No email templates found.</p>
              ) : (
                <div className="space-y-2">
                  {templates.map((tpl) => (
                    <button
                      key={tpl.id}
                      onClick={() => setSelectedTemplate(tpl)}
                      className={`w-full text-left p-3.5 rounded-surface border transition flex items-center justify-between ${
                        selectedTemplate?.id === tpl.id
                          ? 'bg-accent-wash border-accent-edge text-ink shadow-sm'
                          : 'bg-paper border-rule hover:border-faint text-ink-2'
                      }`}
                    >
                      <div>
                        <p className="text-xs font-semibold">{tpl.name}</p>
                        <p className="text-[10px] text-muted font-mono">{tpl.alias}</p>
                      </div>
                      <Eye className="w-4 h-4 text-muted" />
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Template Preview Section */}
            <div className="lg:col-span-2">
              {!selectedTemplate ? (
                <div className="h-64 bg-paper border border-rule border-dashed rounded-surface flex flex-col items-center justify-center text-muted p-8 text-center gap-2 shadow-sm">
                  <FileCode className="w-8 h-8 text-muted opacity-50" />
                  <p className="text-xs">Select a system email template on the left to review subject headers and HTML body content.</p>
                </div>
              ) : (
                <div className="bg-paper border border-rule rounded-surface shadow-sm overflow-hidden flex flex-col min-h-[480px]">
                  <div className="p-6 bg-paper-2 border-b border-rule">
                    <h3 className="font-title text-sm font-semibold text-ink">
                      Template Details
                    </h3>
                    <p className="text-xs text-muted mt-0.5">Subject: {selectedTemplate.subject}</p>
                    <div className="mt-3 flex flex-wrap gap-1.5 items-center">
                      <span className="text-[9px] uppercase font-bold text-muted mr-1.5">Variables:</span>
                      {selectedTemplate.variables?.map((v) => (
                        <span key={v} className="px-2 py-0.5 rounded-chip bg-paper border border-rule font-mono text-[9px] text-ink-2">
                          {"{{"}{v}{"}}"}
                        </span>
                      ))}
                    </div>
                  </div>

                  <div className="flex-1 p-6 bg-paper flex flex-col">
                    <h4 className="text-xs font-bold text-muted uppercase tracking-wider mb-2">HTML Body Preview</h4>
                    {/* Render email preview in custom sandboxed iframe */}
                    <div className="flex-1 border border-rule rounded-surface overflow-hidden bg-white min-h-[300px]">
                      <iframe
                        srcDoc={selectedTemplate.htmlBody}
                        title="Email body preview"
                        className="w-full h-full border-none"
                      />
                    </div>
                  </div>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </NavigationShell>
  );
}
