/*
 * OPL DNS Plugin for BIND 9
 * 
 * This plugin intercepts DNS queries and checks them against the
 * Online Picket Line API to detect labor disputes. If a domain
 * is involved in a labor dispute, the DNS response is modified
 * to point to a block page.
 */

#ifndef OPL_PLUGIN_H
#define OPL_PLUGIN_H

#include <isc/types.h>
#include <dns/types.h>

#define OPL_PLUGIN_VERSION "1.0.0"
#define OPL_PLUGIN_NAME "opl-dns-plugin"

/* Configuration structure */
typedef struct opl_config {
    char *api_endpoint;
    char *block_page_ip;
    int api_timeout;
    int cache_ttl;
    int enabled;
} opl_config_t;

/* Plugin context structure */
typedef struct opl_context {
    opl_config_t config;
    void *cache;
    isc_mem_t *mctx;
} opl_context_t;

/* Plugin initialization */
isc_result_t opl_plugin_init(opl_context_t **ctxp, isc_mem_t *mctx, const char *config_file);

/* Plugin cleanup */
void opl_plugin_destroy(opl_context_t **ctxp);

/* Check domain against OPL API */
isc_result_t opl_check_domain(opl_context_t *ctx, const char *domain, char **dispute_info);

/* Modify DNS response */
isc_result_t opl_modify_response(opl_context_t *ctx, dns_message_t *message, const char *domain);

#endif /* OPL_PLUGIN_H */
