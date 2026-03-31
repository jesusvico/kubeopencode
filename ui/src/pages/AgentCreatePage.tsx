import React, { useState, useMemo, useEffect } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useQuery, useMutation } from '@tanstack/react-query';
import api, { CreateAgentRequest } from '../api/client';
import { useToast } from '../contexts/ToastContext';
import { useNamespace } from '../contexts/NamespaceContext';
import Breadcrumbs from '../components/Breadcrumbs';

function AgentCreatePage() {
  const navigate = useNavigate();
  const { addToast } = useToast();
  const [searchParams] = useSearchParams();
  const { namespace: globalNamespace, isAllNamespaces } = useNamespace();
  const [name, setName] = useState('');
  const [profile, setProfile] = useState('');
  const [workspaceDir, setWorkspaceDir] = useState('');
  const [serviceAccountName, setServiceAccountName] = useState('');
  // selectedTemplate stores "namespace/name" or "" for no template
  const [selectedTemplate, setSelectedTemplate] = useState('');

  const { data: allTemplatesData } = useQuery({
    queryKey: ['all-templates'],
    queryFn: () => api.listAllAgentTemplates({ limit: 200, sortOrder: 'asc' }),
  });

  const allTemplates = useMemo(
    () => allTemplatesData?.templates || [],
    [allTemplatesData]
  );

  // Derive namespace from template selection, or fall back to global
  const namespace = useMemo(() => {
    if (selectedTemplate) {
      const ns = selectedTemplate.split('/')[0];
      if (ns) return ns;
    }
    return isAllNamespaces ? 'default' : globalNamespace;
  }, [selectedTemplate, globalNamespace, isAllNamespaces]);

  // Fetch template details when selected to show inherited values
  const templateParts = selectedTemplate.split('/');
  const { data: templateDetail } = useQuery({
    queryKey: ['agent-template', templateParts[0], templateParts[1]],
    queryFn: () => api.getAgentTemplate(templateParts[0], templateParts[1]),
    enabled: templateParts.length === 2 && !!templateParts[0] && !!templateParts[1],
  });

  const hasTemplate = !!selectedTemplate && !!templateDetail;

  useEffect(() => {
    const templateParam = searchParams.get('template');
    if (templateParam) {
      setSelectedTemplate(templateParam);
    }
  }, [searchParams]);

  // When template changes, clear overrides so inherited values show through
  useEffect(() => {
    setWorkspaceDir('');
    setServiceAccountName('');
  }, [selectedTemplate]);

  const createMutation = useMutation({
    mutationFn: (agent: CreateAgentRequest) => api.createAgent(namespace, agent),
    onSuccess: (agent) => {
      addToast(`Agent "${agent.name}" created successfully`, 'success');
      navigate(`/agents/${agent.namespace}/${agent.name}`);
    },
    onError: (err: Error) => {
      addToast(`Failed to create agent: ${err.message}`, 'error');
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    const agent: CreateAgentRequest = { name };

    if (profile) {
      agent.profile = profile;
    }

    if (selectedTemplate) {
      const templateName = selectedTemplate.split('/')[1];
      if (templateName) {
        agent.templateRef = { name: templateName };
      }
    }

    // Only send workspaceDir/serviceAccountName if user explicitly set them (override)
    if (workspaceDir) {
      agent.workspaceDir = workspaceDir;
    }
    if (serviceAccountName) {
      agent.serviceAccountName = serviceAccountName;
    }

    createMutation.mutate(agent);
  };

  // Validation: name is always required; workspaceDir and serviceAccountName
  // are required only when no template provides them
  const effectiveWorkspaceDir = workspaceDir || templateDetail?.workspaceDir || '';
  const effectiveServiceAccount = serviceAccountName || templateDetail?.serviceAccountName || '';
  const isValid = name && (hasTemplate || (effectiveWorkspaceDir && effectiveServiceAccount));

  return (
    <div className="animate-fade-in">
      <Breadcrumbs items={[
        { label: 'Agents', to: '/agents' },
        { label: 'Create Agent' },
      ]} />

      <div className="bg-white rounded-xl border border-stone-200 overflow-hidden shadow-sm max-w-3xl">
        <div className="px-6 py-5 border-b border-stone-100">
          <h2 className="font-display text-xl font-bold text-stone-900">Create Agent</h2>
          <p className="text-sm text-stone-400 mt-0.5">Create a new AI agent configuration</p>
        </div>

        <form onSubmit={handleSubmit} className="px-6 py-5 space-y-5">
          <div>
            <label
              htmlFor="template"
              className="block text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider mb-1.5"
            >
              Template <span className="normal-case tracking-normal text-stone-300">(optional)</span>
            </label>
            <select
              id="template"
              value={selectedTemplate}
              onChange={(e) => setSelectedTemplate(e.target.value)}
              className="block w-full rounded-lg border-stone-200 bg-white shadow-sm focus:border-primary-500 focus:ring-primary-500 text-sm text-stone-700"
            >
              <option value="">No template</option>
              {allTemplates.map((tmpl) => (
                <option key={`${tmpl.namespace}/${tmpl.name}`} value={`${tmpl.namespace}/${tmpl.name}`}>
                  {tmpl.namespace}/{tmpl.name}
                </option>
              ))}
            </select>
            <p className="mt-1.5 text-xs text-stone-400">
              {allTemplates.length === 0
                ? 'No templates available.'
                : `Agent will be created in namespace "${namespace}".`}
            </p>
          </div>

          <div>
            <label
              htmlFor="name"
              className="block text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider mb-1.5"
            >
              Name
            </label>
            <input
              type="text"
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              placeholder="my-agent"
              className="block w-full rounded-lg border-stone-200 shadow-sm focus:border-primary-500 focus:ring-primary-500 text-sm text-stone-700 placeholder:text-stone-300"
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label
                htmlFor="workspaceDir"
                className="block text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider mb-1.5"
              >
                Workspace Directory
                {hasTemplate && (
                  <span className="normal-case tracking-normal text-stone-300"> (inherited from template)</span>
                )}
              </label>
              <input
                type="text"
                id="workspaceDir"
                value={workspaceDir}
                onChange={(e) => setWorkspaceDir(e.target.value)}
                required={!hasTemplate}
                placeholder={hasTemplate && templateDetail?.workspaceDir ? templateDetail.workspaceDir : '/workspace'}
                className="block w-full rounded-lg border-stone-200 shadow-sm focus:border-primary-500 focus:ring-primary-500 text-sm text-stone-700 font-mono placeholder:text-stone-300 placeholder:font-body"
              />
            </div>

            <div>
              <label
                htmlFor="serviceAccountName"
                className="block text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider mb-1.5"
              >
                Service Account
                {hasTemplate && (
                  <span className="normal-case tracking-normal text-stone-300"> (inherited from template)</span>
                )}
              </label>
              <input
                type="text"
                id="serviceAccountName"
                value={serviceAccountName}
                onChange={(e) => setServiceAccountName(e.target.value)}
                required={!hasTemplate}
                placeholder={hasTemplate && templateDetail?.serviceAccountName ? templateDetail.serviceAccountName : 'default'}
                className="block w-full rounded-lg border-stone-200 shadow-sm focus:border-primary-500 focus:ring-primary-500 text-sm text-stone-700 font-mono placeholder:text-stone-300 placeholder:font-body"
              />
            </div>
          </div>

          <div>
            <label
              htmlFor="profile"
              className="block text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider mb-1.5"
            >
              Profile <span className="normal-case tracking-normal text-stone-300">(optional)</span>
            </label>
            <textarea
              id="profile"
              value={profile}
              onChange={(e) => setProfile(e.target.value)}
              rows={3}
              placeholder="Describe this agent's purpose and capabilities..."
              className="block w-full rounded-lg border-stone-200 shadow-sm focus:border-primary-500 focus:ring-primary-500 text-sm text-stone-700 placeholder:text-stone-300"
            />
          </div>

          {createMutation.isError && (
            <div className="bg-red-50 border border-red-200 rounded-lg p-4">
              <p className="text-red-700 text-sm">
                {(createMutation.error as Error).message}
              </p>
            </div>
          )}

          <div className="flex justify-end space-x-3 pt-2">
            <Link
              to="/agents"
              className="px-4 py-2.5 text-sm font-medium text-stone-600 bg-stone-100 rounded-lg hover:bg-stone-200 transition-colors"
            >
              Cancel
            </Link>
            <button
              type="submit"
              disabled={createMutation.isPending || !isValid}
              className="px-5 py-2.5 text-sm font-medium text-white bg-primary-600 rounded-lg hover:bg-primary-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              {createMutation.isPending ? 'Creating...' : 'Create Agent'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

export default AgentCreatePage;
