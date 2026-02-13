package com.radael.condoguard;

import org.junit.jupiter.api.Test;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.context.TestPropertySource;

@SpringBootTest
@TestPropertySource(properties = {
    "spring.data.mongodb.uri=mongodb://localhost:27017/integration-testing-db",
    "jwt.secret=dGVzdF9zZWNyZXRfa2V5X2Zvcl90ZXN0aW5nX3B1cnBvc2VzX29ubHk="
})
class ApiTests {

  @Test
  void contextLoads() {}
}
