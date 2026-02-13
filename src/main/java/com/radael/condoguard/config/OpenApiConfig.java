package com.radael.condoguard.config;

import io.swagger.v3.oas.models.OpenAPI;
import io.swagger.v3.oas.models.info.Info;
import io.swagger.v3.oas.models.info.License;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class OpenApiConfig {

  @Bean
  public OpenAPI condoGuardOpenAPI() {
    return new OpenAPI()
        .info(
            new Info()
                .title("CondoGuard API")
                .description("API para gestão de condomínios")
                .version("v0.0.1")
                .license(
                    new License()
                        .name("GNU General Public License v3.0")
                        .url("https://www.gnu.org/licenses/")));
  }
}
