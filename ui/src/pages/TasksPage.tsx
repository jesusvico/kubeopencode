import React, { useState, useEffect, useMemo } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import api from '../api/client';
import StatusBadge from '../components/StatusBadge';
import Labels from '../components/Labels';
import TimeAgo from '../components/TimeAgo';
import ResourceFilter from '../components/ResourceFilter';
import { TableSkeleton } from '../components/Skeleton';
import { useFilterState } from '../hooks/useFilterState';
import { getNamespaceCookie, setNamespaceCookie } from '../utils/cookies';
import { LABEL_AGENT, appendLabelSelector } from '../utils/labels';

const PAGE_SIZE_OPTIONS = [10, 20, 50];
const ALL_NAMESPACES = '__all__';
const PHASE_OPTIONS = ['', 'Pending', 'Queued', 'Running', 'Completed', 'Failed'];

function TasksPage() {
  const [searchParams] = useSearchParams();
  const [namespace, setNamespace] = useState(() => {
    const urlParam = new URLSearchParams(window.location.search).get('namespace');
    if (urlParam) return urlParam;
    return getNamespaceCookie() || 'default';
  });
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [phaseFilter, setPhaseFilter] = useState('');
  const [agentFilter, setAgentFilter] = useState('');
  const [filters, setFilters] = useFilterState();

  useEffect(() => {
    const namespaceParam = searchParams.get('namespace');
    if (namespaceParam && namespaceParam !== namespace) {
      setNamespace(namespaceParam);
      if (namespaceParam !== ALL_NAMESPACES) {
        setNamespaceCookie(namespaceParam);
      }
    }
  }, [searchParams, namespace]);

  const handleNamespaceChange = (newNamespace: string) => {
    setNamespace(newNamespace);
    if (newNamespace !== ALL_NAMESPACES) {
      setNamespaceCookie(newNamespace);
    }
  };

  useEffect(() => {
    setCurrentPage(1);
  }, [namespace, phaseFilter, agentFilter, filters.name, filters.labelSelector]);

  // Reset agent filter when namespace changes
  useEffect(() => {
    setAgentFilter('');
  }, [namespace]);

  const { data: namespacesData } = useQuery({
    queryKey: ['namespaces'],
    queryFn: () => api.getNamespaces(),
  });

  const isAllNamespaces = namespace === ALL_NAMESPACES;

  const { data: agentsData } = useQuery({
    queryKey: ['agents-for-filter', namespace],
    queryFn: () =>
      isAllNamespaces
        ? api.listAllAgents({ limit: 100, sortOrder: 'asc' })
        : api.listAgents(namespace, { limit: 100, sortOrder: 'asc' }),
    staleTime: 60_000,
  });

  const uniqueAgentNames = useMemo(
    () => agentsData ? [...new Set(agentsData.agents.map((a) => a.name))] : [],
    [agentsData]
  );

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['tasks', namespace, currentPage, pageSize, phaseFilter, agentFilter, filters.name, filters.labelSelector],
    queryFn: () => {
      let labelSelector = filters.labelSelector || '';
      if (agentFilter) {
        labelSelector = appendLabelSelector(labelSelector, `${LABEL_AGENT}=${agentFilter}`);
      }
      const params = {
        limit: pageSize,
        offset: (currentPage - 1) * pageSize,
        sortOrder: 'desc' as const,
        name: filters.name || undefined,
        labelSelector: labelSelector || undefined,
        phase: phaseFilter || undefined,
      };
      return isAllNamespaces
        ? api.listAllTasks(params)
        : api.listTasks(namespace, params);
    },
    refetchInterval: 5000,
  });

  return (
    <div className="animate-fade-in">
      <div className="sm:flex sm:items-center sm:justify-between mb-6">
        <div>
          <h2 className="font-display text-2xl font-bold text-stone-900 tracking-tight">Tasks</h2>
          <p className="mt-1 text-sm text-stone-500">
            Manage and monitor AI agent tasks
          </p>
        </div>
        <div className="mt-4 sm:mt-0 flex items-center space-x-3">
          <select
            value={namespace}
            onChange={(e) => handleNamespaceChange(e.target.value)}
            className="block w-48 rounded-lg border-stone-200 bg-white shadow-sm focus:border-primary-500 focus:ring-primary-500 text-sm text-stone-700"
          >
            <option value={ALL_NAMESPACES}>All Namespaces</option>
            {namespacesData?.namespaces.map((ns) => (
              <option key={ns} value={ns}>
                {ns}
              </option>
            ))}
          </select>
          <Link
            to={`/tasks/create?namespace=${namespace}`}
            className="inline-flex items-center gap-2 px-4 py-2.5 text-sm font-medium text-white bg-stone-900 rounded-lg hover:bg-stone-800 transition-colors shadow-sm"
          >
            <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M12 5v14M5 12h14" strokeLinecap="round" />
            </svg>
            New Task
          </Link>
        </div>
      </div>

      {/* Filter bar */}
      <div className="mb-4 space-y-3">
        <ResourceFilter
          filters={filters}
          onFilterChange={setFilters}
          placeholder="Filter tasks by name..."
        />
        <div className="flex items-center space-x-3 flex-wrap gap-y-2">
          <div className="flex items-center space-x-1.5">
            {PHASE_OPTIONS.map((phase) => (
              <button
                key={phase || 'all'}
                onClick={() => setPhaseFilter(phase)}
                className={`px-3 py-1.5 text-xs font-medium rounded-lg border transition-colors ${
                  phaseFilter === phase
                    ? 'bg-stone-900 text-white border-stone-900'
                    : 'bg-white text-stone-500 border-stone-200 hover:border-stone-300 hover:text-stone-700'
                }`}
              >
                {phase || 'All'}
              </button>
            ))}
          </div>
          {uniqueAgentNames.length > 0 && (
            <div className="flex items-center space-x-1.5">
              <span className="text-xs text-stone-400">Agent:</span>
              <select
                value={agentFilter}
                onChange={(e) => setAgentFilter(e.target.value)}
                className="block w-40 rounded-lg border-stone-200 bg-white shadow-sm focus:border-primary-500 focus:ring-primary-500 text-xs text-stone-700 py-1.5"
              >
                <option value="">All Agents</option>
                {uniqueAgentNames.map((name) => (
                  <option key={name} value={name}>{name}</option>
                ))}
              </select>
            </div>
          )}
        </div>
      </div>

      {isLoading ? (
        <div className="bg-white rounded-xl border border-stone-200 overflow-hidden shadow-sm">
          <TableSkeleton rows={5} cols={isAllNamespaces ? 7 : 6} />
        </div>
      ) : error ? (
        <div className="bg-red-50 border border-red-200 rounded-xl p-5">
          <p className="text-red-700 text-sm">Error loading tasks: {(error as Error).message}</p>
          <button
            onClick={() => refetch()}
            className="mt-2 text-sm text-red-600 hover:text-red-800 font-medium"
          >
            Retry
          </button>
        </div>
      ) : (
        <div className="bg-white rounded-xl border border-stone-200 overflow-hidden shadow-sm">
          <table className="min-w-full divide-y divide-stone-100">
            <thead className="bg-stone-50/60">
              <tr>
                <th className="px-5 py-3 text-left text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider">
                  Name
                </th>
                {isAllNamespaces && (
                  <th className="px-5 py-3 text-left text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider">
                    Namespace
                  </th>
                )}
                <th className="px-5 py-3 text-left text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider">
                  Status
                </th>
                <th className="px-5 py-3 text-left text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider hidden lg:table-cell">
                  Labels
                </th>
                <th className="px-5 py-3 text-left text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider">
                  Agent
                </th>
                <th className="px-5 py-3 text-left text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider hidden sm:table-cell">
                  Duration
                </th>
                <th className="px-5 py-3 text-left text-[11px] font-display font-medium text-stone-400 uppercase tracking-wider">
                  Created
                </th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-stone-100">
              {data?.tasks.length === 0 ? (
                <tr>
                  <td colSpan={isAllNamespaces ? 7 : 6} className="px-5 py-12 text-center text-stone-400 text-sm">
                    No tasks found.{' '}
                    {!isAllNamespaces && (
                      <Link to={`/tasks/create?namespace=${namespace}`} className="text-primary-600 hover:text-primary-700 font-medium">
                        Create your first task
                      </Link>
                    )}
                  </td>
                </tr>
              ) : (
                data?.tasks.map((task) => (
                  <tr key={`${task.namespace}/${task.name}`} className="hover:bg-stone-50/60 transition-colors">
                    <td className="px-5 py-3.5 whitespace-nowrap">
                      <Link
                        to={`/tasks/${task.namespace}/${task.name}`}
                        className="text-stone-800 hover:text-primary-600 font-medium text-sm transition-colors"
                      >
                        {task.name}
                      </Link>
                    </td>
                    {isAllNamespaces && (
                      <td className="px-5 py-3.5 whitespace-nowrap text-sm text-stone-400">
                        {task.namespace}
                      </td>
                    )}
                    <td className="px-5 py-3.5 whitespace-nowrap">
                      <StatusBadge phase={task.phase || 'Pending'} />
                    </td>
                    <td className="px-5 py-3.5 hidden lg:table-cell">
                      <Labels labels={task.labels} maxDisplay={2} />
                    </td>
                    <td className="px-5 py-3.5 whitespace-nowrap text-sm text-stone-400 font-mono text-xs">
                      {task.agentRef?.name || 'default'}
                    </td>
                    <td className="px-5 py-3.5 whitespace-nowrap text-sm text-stone-400 hidden sm:table-cell font-mono text-xs">
                      {task.duration || '-'}
                    </td>
                    <td className="px-5 py-3.5 whitespace-nowrap text-xs text-stone-400">
                      <TimeAgo date={task.createdAt} />
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>

          {/* Pagination */}
          {data?.pagination && data.pagination.totalCount > 0 && (
            <div className="bg-white px-5 py-3 flex items-center justify-between border-t border-stone-100">
              <div className="flex-1 flex justify-between sm:hidden">
                <button
                  onClick={() => setCurrentPage(p => Math.max(1, p - 1))}
                  disabled={currentPage === 1}
                  className="px-3 py-1.5 text-sm font-medium text-stone-600 bg-stone-100 rounded-lg hover:bg-stone-200 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                >
                  Previous
                </button>
                <button
                  onClick={() => setCurrentPage(p => p + 1)}
                  disabled={!data.pagination.hasMore}
                  className="ml-3 px-3 py-1.5 text-sm font-medium text-stone-600 bg-stone-100 rounded-lg hover:bg-stone-200 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                >
                  Next
                </button>
              </div>
              <div className="hidden sm:flex-1 sm:flex sm:items-center sm:justify-between">
                <div className="flex items-center space-x-4">
                  <p className="text-xs text-stone-400">
                    <span className="font-medium text-stone-600">{data.pagination.offset + 1}</span>
                    {' '}-{' '}
                    <span className="font-medium text-stone-600">
                      {Math.min(data.pagination.offset + data.tasks.length, data.pagination.totalCount)}
                    </span>
                    {' '}of{' '}
                    <span className="font-medium text-stone-600">{data.pagination.totalCount}</span>
                  </p>
                  <select
                    value={pageSize}
                    onChange={(e) => {
                      setPageSize(Number(e.target.value));
                      setCurrentPage(1);
                    }}
                    className="block w-16 rounded-lg border-stone-200 text-xs text-stone-600 focus:border-primary-500 focus:ring-primary-500"
                  >
                    {PAGE_SIZE_OPTIONS.map((size) => (
                      <option key={size} value={size}>{size}</option>
                    ))}
                  </select>
                </div>
                <div className="flex items-center space-x-1">
                  <button
                    onClick={() => setCurrentPage(p => Math.max(1, p - 1))}
                    disabled={currentPage === 1}
                    className="px-3 py-1.5 text-xs font-medium text-stone-500 bg-stone-50 border border-stone-200 rounded-lg hover:bg-stone-100 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                  >
                    Prev
                  </button>
                  <span className="px-3 py-1.5 text-xs font-mono text-stone-500">
                    {currentPage}/{Math.ceil(data.pagination.totalCount / pageSize)}
                  </span>
                  <button
                    onClick={() => setCurrentPage(p => p + 1)}
                    disabled={!data.pagination.hasMore}
                    className="px-3 py-1.5 text-xs font-medium text-stone-500 bg-stone-50 border border-stone-200 rounded-lg hover:bg-stone-100 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                  >
                    Next
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export default TasksPage;
