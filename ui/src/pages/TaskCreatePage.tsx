import React, { useState, useMemo, useEffect } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useQuery, useMutation } from '@tanstack/react-query';
import api, { CreateTaskRequest } from '../api/client';
import { useToast } from '../contexts/ToastContext';
import Breadcrumbs from '../components/Breadcrumbs';

function TaskCreatePage() {
  const navigate = useNavigate();
  const { addToast } = useToast();
  const [searchParams] = useSearchParams();
  const [namespace, setNamespace] = useState('default');
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [selectedAgent, setSelectedAgent] = useState('');

  const { data: namespacesData } = useQuery({
    queryKey: ['namespaces'],
    queryFn: () => api.getNamespaces(),
  });

  const { data: agentsData } = useQuery({
    queryKey: ['agents'],
    queryFn: () => api.listAllAgents(),
  });

  const rerunTaskName = searchParams.get('rerun');
  const rerunNamespace = searchParams.get('namespace') || 'default';
  const { data: rerunTask } = useQuery({
    queryKey: ['task', rerunNamespace, rerunTaskName],
    queryFn: () => api.getTask(rerunNamespace, rerunTaskName!),
    enabled: !!rerunTaskName,
  });

  useEffect(() => {
    const namespaceParam = searchParams.get('namespace');
    if (namespaceParam) {
      setNamespace(namespaceParam);
    }
    const agentParam = searchParams.get('agent');
    if (agentParam) {
      setSelectedAgent(agentParam);
      const agentNs = agentParam.split('/')[0];
      if (agentNs) {
        setNamespace(agentNs);
      }
    }
  }, [searchParams]);

  useEffect(() => {
    if (rerunTask) {
      if (rerunTask.description) {
        setDescription(rerunTask.description);
      }
      if (rerunTask.agentRef) {
        const agentKey = `${rerunTask.namespace}/${rerunTask.agentRef.name}`;
        setSelectedAgent(agentKey);
      }
    }
  }, [rerunTask]);

  const availableAgents = useMemo(() => {
    if (!agentsData?.agents) return [];
    return agentsData.agents.filter((agent) => agent.namespace === namespace);
  }, [agentsData?.agents, namespace]);

  const handleNamespaceChange = (newNamespace: string) => {
    setNamespace(newNamespace);
    if (selectedAgent) {
      const agent = agentsData?.agents.find(
        (a) => `${a.namespace}/${a.name}` === selectedAgent
      );
      if (agent && agent.namespace !== newNamespace) {
        setSelectedAgent('');
      }
    }
  };

  const createMutation = useMutation({
    mutationFn: (task: CreateTaskRequest) => api.createTask(namespace, task),
    onSuccess: (task) => {
      addToast(`Task "${task.name}" created successfully`, 'success');
      navigate(`/tasks/${task.namespace}/${task.name}`);
    },
    onError: (err: Error) => {
      addToast(`Failed to create task: ${err.message}`, 'error');
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    const task: CreateTaskRequest = {};

    if (name) {
      task.name = name;
    }

    if (description) {
      task.description = description;
    }

    if (selectedAgent) {
      const agent = agentsData?.agents.find(
        (a) => `${a.namespace}/${a.name}` === selectedAgent
      );
      if (agent) {
        task.agentRef = {
          name: agent.name,
        };
      }
    }

    createMutation.mutate(task);
  };

  const isValid = description && selectedAgent;

  return (
    <div className="animate-fade-in">
      <Breadcrumbs items={[
        { label: 'Tasks', to: `/tasks?namespace=${namespace}` },
        { label: 'Create Task' },
      ]} />

      <div className="bg-white rounded-xl border border-stone-200 overflow-hidden shadow-sm max-w-3xl">
        <div className="px-6 py-5 border-b border-stone-100">
          <h2 className="font-display text-xl font-bold text-stone-900">Create Task</h2>
          <p className="text-sm text-stone-400 mt-0.5">Create a new AI agent task</p>
        </div>

        <form onSubmit={handleSubmit} className="px-6 py-5 space-y-5">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label
                htmlFor="namespace"
                className="block text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider mb-1.5"
              >
                Namespace
              </label>
              <select
                id="namespace"
                value={namespace}
                onChange={(e) => handleNamespaceChange(e.target.value)}
                className="block w-full rounded-lg border-stone-200 bg-white shadow-sm focus:border-primary-500 focus:ring-primary-500 text-sm text-stone-700"
              >
                {namespacesData?.namespaces.map((ns) => (
                  <option key={ns} value={ns}>
                    {ns}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label
                htmlFor="name"
                className="block text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider mb-1.5"
              >
                Name <span className="normal-case tracking-normal text-stone-300">(optional)</span>
              </label>
              <input
                type="text"
                id="name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Auto-generated if empty"
                className="block w-full rounded-lg border-stone-200 shadow-sm focus:border-primary-500 focus:ring-primary-500 text-sm text-stone-700 placeholder:text-stone-300"
              />
            </div>
          </div>

          <div>
            <label
              htmlFor="agent"
              className="block text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider mb-1.5"
            >
              Agent
            </label>
            <select
              id="agent"
              value={selectedAgent}
              onChange={(e) => setSelectedAgent(e.target.value)}
              required
              className="block w-full rounded-lg border-stone-200 bg-white shadow-sm focus:border-primary-500 focus:ring-primary-500 text-sm text-stone-700"
            >
              <option value="">
                {availableAgents.length === 0
                  ? 'No agents available'
                  : 'Select an agent...'}
              </option>
              {availableAgents.map((agent) => (
                <option
                  key={`${agent.namespace}/${agent.name}`}
                  value={`${agent.namespace}/${agent.name}`}
                >
                  {agent.namespace}/{agent.name}
                </option>
              ))}
            </select>
            <p className="mt-1.5 text-xs text-stone-400">
              {availableAgents.length === 0
                ? 'No agents available for this namespace.'
                : `${availableAgents.length} agent${availableAgents.length !== 1 ? 's' : ''} available`}
            </p>
            {selectedAgent && agentsData?.agents.find(
              (a) => `${a.namespace}/${a.name}` === selectedAgent
            )?.mode === 'Server' && (
              <div className="mt-2 flex items-start gap-2 bg-violet-50 border border-violet-200 rounded-lg px-3 py-2">
                <svg className="w-4 h-4 text-violet-500 mt-0.5 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <circle cx="12" cy="12" r="10" />
                  <path d="M12 16v-4M12 8h.01" strokeLinecap="round" />
                </svg>
                <p className="text-xs text-violet-700">
                  <span className="font-medium">Server mode agent.</span> Task contexts are not supported — the task will use the agent's pre-loaded workspace. For interactive sessions, use <code className="bg-violet-100 px-1 py-0.5 rounded font-mono">opencode attach</code> instead.
                </p>
              </div>
            )}
          </div>

          <div>
            <label
              htmlFor="description"
              className="block text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider mb-1.5"
            >
              Task Prompt
            </label>
            <textarea
              id="description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={12}
              required
              placeholder="Describe what you want the AI agent to do..."
              className="block w-full rounded-lg border-stone-200 shadow-sm focus:border-primary-500 focus:ring-primary-500 text-sm text-stone-700 font-mono placeholder:text-stone-300 placeholder:font-body"
            />
            <p className="mt-1.5 text-xs text-stone-400">
              This will be the main instruction for the AI agent
            </p>
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
              to={`/tasks?namespace=${namespace}`}
              className="px-4 py-2.5 text-sm font-medium text-stone-600 bg-stone-100 rounded-lg hover:bg-stone-200 transition-colors"
            >
              Cancel
            </Link>
            <button
              type="submit"
              disabled={createMutation.isPending || !isValid}
              className="px-5 py-2.5 text-sm font-medium text-white bg-stone-900 rounded-lg hover:bg-stone-800 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              {createMutation.isPending ? 'Creating...' : 'Create Task'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

export default TaskCreatePage;
